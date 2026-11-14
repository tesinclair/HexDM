// Package gfx is the rendered greeter: SDL3 for the screen and keyboard,
// OpenGL ES for the drawing. On a console it draws straight to the display
// (no X or Wayland underneath); when debugging on a desktop it opens a
// window.
package gfx

/*
#cgo pkg-config: sdl3 glesv2
#include <stdlib.h>
#include "hx.h"
*/
import "C"

import (
	"embed"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"math/rand"
	"os"
	"runtime"
	"time"
	"unsafe"

	"github.com/tesinclair/hexdm/greeter"
	"github.com/tesinclair/hexdm/settings"
)

// ErrUnavailable means the screen could not be opened at all. Nothing was
// shown and no input was taken, so another greeter can take over.
var ErrUnavailable = errors.New("rendered greeter unavailable")

//go:embed shaders
var shaders embed.FS

// ScriptStep presses a key some time after the greeter first appears.
type ScriptStep struct {
	At  time.Duration
	Key greeter.Key
}

type Options struct {
	// Open a window on the desktop this was started from, instead of
	// taking the display. For debugging.
	Windowed      bool
	Width, Height int

	// Write one frame to SnapshotPath as a PNG, SnapshotAt after the
	// greeter appears, then stop. Nothing is shown on screen.
	SnapshotPath string
	SnapshotAt   time.Duration
	Script       []ScriptStep
}

type renderer struct {
	cfg  settings.Config
	opt  Options
	sc   *scene
	bg   string
	open bool

	openedAt time.Time
	started  time.Time
	last     time.Time
}

func shader(name string) string {
	src, err := shaders.ReadFile("shaders/" + name)
	if err != nil {
		panic(err) // the files are compiled in
	}
	return string(src)
}

func program(slot C.int, vs, fs string) error {
	cvs, cfs := C.CString(vs), C.CString(fs)
	defer C.free(unsafe.Pointer(cvs))
	defer C.free(unsafe.Pointer(cfs))
	var msg [1024]C.char
	if C.hx_program(slot, cvs, cfs, &msg[0], C.int(len(msg))) == 0 {
		return errors.New(C.GoString(&msg[0]))
	}
	return nil
}

// start opens the screen and gets everything ready to draw.
func (r *renderer) start() error {
	opts := C.hx_opts{
		kms:       C.int(flag(!r.opt.Windowed)),
		hidden:    C.int(flag(r.opt.SnapshotPath != "")),
		width:     C.int(r.opt.Width),
		height:    C.int(r.opt.Height),
		secondary: C.int(flag(r.cfg.SecondaryDisplays)),
	}
	var msg [1024]C.char
	if C.hx_init(&opts, &msg[0], C.int(len(msg))) == 0 {
		return errors.New(C.GoString(&msg[0]))
	}

	vs := shader("fullscreen.vert")
	programs := []struct {
		slot   C.int
		name   string
		vs, fs string
	}{
		{C.HX_PROG_BG, "background " + r.bg, vs, shader("bg_common.glsl") + shader("bg_"+r.bg+".frag")},
		{C.HX_PROG_BATCH, "batch", shader("batch.vert"), shader("batch.frag")},
		{C.HX_PROG_BRIGHT, "bright", vs, shader("bright.frag")},
		{C.HX_PROG_BLUR, "blur", vs, shader("blur.frag")},
		{C.HX_PROG_POST, "post", vs, shader("post.frag")},
	}
	for _, p := range programs {
		if err := program(p.slot, p.vs, p.fs); err != nil {
			C.hx_shutdown()
			return fmt.Errorf("%s shader: %w", p.name, err)
		}
	}

	now := time.Now()
	if r.started.IsZero() {
		r.started = now
	}
	r.openedAt, r.last = now, now
	r.sc.txt.dirty = true // a new screen has not seen the glyphs yet
	r.open = true
	return nil
}

// stop gives the screen and the keyboard back.
func (r *renderer) stop() {
	if r.open {
		C.hx_shutdown()
		r.open = false
	}
}

// keys passes everything typed since the last frame to the state.
func (r *renderer) keys(s *greeter.State, now time.Time) {
	var ev C.hx_event
	for C.hx_poll(&ev) != 0 {
		switch ev._type {
		case C.HX_EV_QUIT:
			// Only a window can be closed.
			s.Key(greeter.Key{Type: greeter.KeyInterrupt}, now)
		case C.HX_EV_TEXT:
			s.Key(greeter.Key{Type: greeter.KeyText, Text: C.GoString(&ev.text[0])}, now)
		case C.HX_EV_KEY:
			types := map[C.int]greeter.KeyType{
				C.HX_KEY_ENTER:     greeter.KeyEnter,
				C.HX_KEY_ESC:       greeter.KeyEsc,
				C.HX_KEY_BACKSPACE: greeter.KeyBackspace,
				C.HX_KEY_LEFT:      greeter.KeyLeft,
				C.HX_KEY_RIGHT:     greeter.KeyRight,
				C.HX_KEY_UP:        greeter.KeyUp,
				C.HX_KEY_DOWN:      greeter.KeyDown,
				C.HX_KEY_INTERRUPT: greeter.KeyInterrupt,
			}
			if t, ok := types[ev.key]; ok {
				s.Key(greeter.Key{Type: t}, now)
			}
		}
		// The event may have held a typed character.
		ev = C.hx_event{}
	}
}

// frame draws the screen once. With capture it returns the picture instead
// of showing it.
func (r *renderer) frame(s *greeter.State, now time.Time, capture bool) *image.NRGBA {
	var cw, ch C.int
	C.hx_size(&cw, &ch)
	w, h := int(cw), int(ch)

	dt := min(float32(now.Sub(r.last).Seconds()), 0.1)
	r.last = now
	age := float32(now.Sub(r.openedAt).Seconds())
	t := float32(now.Sub(r.started).Seconds())

	lk := r.sc.draw(s, now, w, h, age, t, dt)

	if txt := r.sc.txt; txt.dirty {
		C.hx_atlas((*C.uchar)(unsafe.Pointer(&txt.atlas[0])), atlasSize, atlasSize)
		txt.dirty = false
	}

	pal := r.sc.pal
	vec := func(c rgba) [3]C.float { return [3]C.float{C.float(c.r), C.float(c.g), C.float(c.b)} }
	C.hx_begin(&C.hx_frame{
		time:      C.float(t),
		base:      vec(pal.base),
		primary:   vec(pal.primary),
		accent:    vec(lk.accent),
		energy:    C.float(lk.energy),
		fade:      C.float(lk.fade),
		glitch:    C.float(lk.glitch),
		scanlines: C.float(r.cfg.Scanlines),
		vignette:  C.float(r.cfg.Vignette),
		bloom:     C.float(r.cfg.Bloom),
	})
	if n := r.sc.b.verts(); n > 0 {
		C.hx_batch((*C.float)(unsafe.Pointer(&r.sc.b.v[0])), C.int(n))
	}

	if !capture {
		C.hx_end(C.int(flag(r.opt.SnapshotPath == "")), nil)
		return nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	C.hx_end(0, (*C.uchar)(unsafe.Pointer(&img.Pix[0])))
	// OpenGL's first row is the bottom one.
	row := make([]byte, img.Stride)
	for y := 0; y < h/2; y++ {
		top, bottom := img.Pix[y*img.Stride:(y+1)*img.Stride], img.Pix[(h-1-y)*img.Stride:(h-y)*img.Stride]
		copy(row, top)
		copy(top, bottom)
		copy(bottom, row)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0xFF
	}
	return img
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Run shows the rendered greeter until main shuts it down, the session ends
// or (when debugging) the user quits.
//
// If the screen can't be opened it returns ErrUnavailable straight away,
// having consumed nothing from events. Once a login is accepted it plays
// out, lets go of the display and the keyboard, and tells main on
// link.Released; it then waits, off screen, for the session to end.
func Run(s *greeter.State, link *greeter.Link, events <-chan greeter.Event, cfg settings.Config, opt Options) error {
	// SDL and the OpenGL context belong to one thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	sc, err := newScene(cfg)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	r := &renderer{cfg: cfg, opt: opt, sc: sc, bg: cfg.Background}
	if r.bg == "random" {
		r.bg = settings.Backgrounds[rand.Intn(len(settings.Backgrounds)-1)] // any but plain
	}
	if r.opt.Width <= 0 || r.opt.Height <= 0 {
		r.opt.Width, r.opt.Height = 1600, 900
	}
	if err := r.start(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer r.stop()
	log.Printf("Rendered greeter up. Background: %s.", r.bg)

	script := opt.Script
	for {
		if !r.open {
			// The session has the screen. Wait for main to say how it
			// went.
			s.Handle(<-events, time.Now())
			if s.Done {
				return s.ExitErr
			}
			if s.SessionFailed {
				if err := r.start(); err != nil {
					log.Printf("Could not reopen the screen to report a failed session: %v", err)
				}
			}
			continue
		}

		now := time.Now()
		r.keys(s, now)
		for len(script) > 0 && now.Sub(r.started) >= script[0].At {
			s.Key(script[0].Key, now)
			script = script[1:]
		}
	drain:
		for {
			select {
			case ev := <-events:
				s.Handle(ev, now)
			default:
				break drain
			}
		}
		s.Tick(now)

		if s.Done {
			return s.ExitErr
		}
		if s.Welcome() && now.Sub(s.AcceptedAt) >= exitAfter {
			r.stop()
			link.Release()
			continue
		}

		if opt.SnapshotPath != "" && now.Sub(r.started) >= opt.SnapshotAt {
			return savePNG(opt.SnapshotPath, r.frame(s, now, true))
		}
		r.frame(s, now, false)

		// Showing a frame normally waits for the display. If it didn't
		// (or nothing is shown at all), don't spin.
		if spent := time.Since(now); spent < 4*time.Millisecond {
			time.Sleep(8*time.Millisecond - spent)
		}
	}
}
