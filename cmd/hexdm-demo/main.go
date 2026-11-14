// hexdm-demo shows the greeter in a window with a pretend system behind it:
// made-up users, no PAM, no root, and nothing is ever started. It is for
// working on how the greeter looks and behaves.
//
//	password "hex"     logs in
//	password "crash"   logs in, then the session fails to start
//	anything else      is rejected
//	user "rincewind"   has a fingerprint enrolled; every scan is rejected
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tesinclair/hexdm/gfx"
	"github.com/tesinclair/hexdm/greeter"
	"github.com/tesinclair/hexdm/settings"
	"github.com/tesinclair/hexdm/tui"
)

const fingerprintUser = "rincewind"

func main() {
	var (
		config     = flag.String("config", "config/hexdm.conf", "greeter config to use")
		background = flag.String("background", "", "background to draw, instead of the one in the config")
		user       = flag.String("user", "", "user to start on")
		size       = flag.String("size", "1600x900", "window size")
		useTUI     = flag.Bool("tui", false, "show the terminal fallback instead")
		snapshot   = flag.String("snapshot", "", "write one frame to this PNG and exit, showing nothing")
		at         = flag.Float64("at", 2, "seconds in at which the snapshot is taken")
		keys       = flag.String("keys", "", `keys to press, as "seconds=key ..." (text, or <enter> <esc> <bs> <left> <right> <up> <down>)`)
	)
	flag.Parse()

	os.Setenv("HEX_DEBUG", "1") // lets q close it

	cfg, problems := settings.Load(*config)
	for _, p := range problems {
		log.Printf("config: %v", p)
	}
	if *background != "" {
		cfg.Background = *background
	}

	users := []string{"ada", fingerprintUser, "ponder"}
	if i := slices.Index(users, *user); i > 0 {
		users = append(users[i:], users[:i]...)
	}
	sessions := []string{"Hyprland", "Sway", "Niri"}

	script, err := parseKeys(*keys)
	if err != nil {
		log.Fatal(err)
	}
	opt := gfx.Options{Windowed: true, SnapshotPath: *snapshot, SnapshotAt: time.Duration(*at * float64(time.Second)), Script: script}
	if _, err := fmt.Sscanf(*size, "%dx%d", &opt.Width, &opt.Height); err != nil {
		log.Fatalf("size %q is not like 1600x900", *size)
	}

	link := &greeter.Link{
		Fprintd:       make(chan bool, 1),
		FprintdTries:  make(chan int, 1),
		ScannerActive: make(chan bool, 1),
		Auth:          make(chan error, 1),
		SessionIdx:    make(chan int, 1),
		Kill:          make(chan bool, 1),
		User:          make(chan string, 1),
		Password:      make(chan []byte, 1),
		Session:       make(chan string, 1),
		Power:         make(chan string, 1),
		Released:      make(chan struct{}, 1),
	}

	sys := &pretend{link: link, tries: map[string]int{fingerprintUser: 3}}
	tries, hasFprint := sys.fprint(users[0])
	link.FprintdTries <- tries
	link.Fprintd <- hasFprint

	state, err := greeter.New(users, sessions, link)
	if err != nil {
		log.Fatal(err)
	}
	events := link.Events()
	go sys.run(users[0])

	if *useTUI {
		err = tui.Run(state, link, events)
	} else {
		err = gfx.Run(state, link, events, cfg, opt)
	}
	if err != nil && !errors.Is(err, greeter.UserQuit) {
		log.Fatal(err)
	}
}

// parseKeys reads "0.5=hello 1=<enter>" into a script.
func parseKeys(spec string) ([]gfx.ScriptStep, error) {
	named := map[string]greeter.KeyType{
		"<enter>": greeter.KeyEnter, "<esc>": greeter.KeyEsc, "<bs>": greeter.KeyBackspace,
		"<left>": greeter.KeyLeft, "<right>": greeter.KeyRight, "<up>": greeter.KeyUp, "<down>": greeter.KeyDown,
	}
	var script []gfx.ScriptStep
	for _, field := range strings.Fields(spec) {
		when, key, ok := strings.Cut(field, "=")
		secs, err := strconv.ParseFloat(when, 64)
		if !ok || err != nil || key == "" {
			return nil, fmt.Errorf("key %q is not like 1.5=<enter>", field)
		}
		at := time.Duration(secs * float64(time.Second))
		if t, ok := named[key]; ok {
			script = append(script, gfx.ScriptStep{At: at, Key: greeter.Key{Type: t}})
			continue
		}
		for _, r := range key {
			script = append(script, gfx.ScriptStep{At: at, Key: greeter.Key{Type: greeter.KeyText, Text: string(r)}})
		}
	}
	slices.SortStableFunc(script, func(a, b gfx.ScriptStep) int { return int(a.At - b.At) })
	return script, nil
}

// pretend stands in for the real main loop.
type pretend struct {
	link  *greeter.Link
	tries map[string]int // fingerprint tries left
}

func (p *pretend) fprint(user string) (tries int, available bool) {
	if n := p.tries[user]; n > 0 {
		return n, true
	}
	return -1, false
}

func (p *pretend) run(user string) {
	link := p.link
	// A finger lands on the reader every few seconds, and is rejected.
	finger := time.NewTicker(6 * time.Second)
	defer finger.Stop()

	for {
		select {
		case user = <-link.User:
			log.Printf("demo: user is now %s", user)
			tries, available := p.fprint(user)
			if available {
				link.FprintdTries <- tries
			}
			link.Fprintd <- available
			link.SessionIdx <- len(user) % 3 // "remembered" session
			finger.Reset(6 * time.Second)

		case name := <-link.Session:
			log.Printf("demo: session is now %s", name)

		case req := <-link.Power:
			log.Printf("demo: would %s", req)

		case <-finger.C:
			if p.tries[user] < 1 {
				continue
			}
			link.ScannerActive <- true
			time.Sleep(1500 * time.Millisecond)
			link.ScannerActive <- false
			p.tries[user]--
			log.Printf("demo: fingerprint rejected, %d tries left", p.tries[user])
			link.Auth <- greeter.ErrFingerprintNoMatch
			link.FprintdTries <- p.tries[user]

		case pass := <-link.Password:
			time.Sleep(900 * time.Millisecond) // PAM takes a moment
			word := string(pass)
			clear(pass)
			if word != "hex" && word != "crash" {
				link.Auth <- errors.New("Invalid Password.")
				continue
			}
			link.Auth <- greeter.LoginAccepted
			<-link.Released
			log.Printf("demo: the greeter let go of the screen; a session for %s would start now", user)
			time.Sleep(1500 * time.Millisecond)
			if word == "crash" {
				link.Auth <- greeter.ErrSessionFailed
				time.Sleep(4 * time.Second)
				link.Kill <- true
				return
			}
			link.Auth <- nil
			return
		}
	}
}
