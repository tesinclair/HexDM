package gfx

import (
	"math"
	"os"
	"strings"
	"time"

	"github.com/tesinclair/hexdm/greeter"
	"github.com/tesinclair/hexdm/settings"
)

// How long the "access granted" animation runs before the screen is handed
// to the session.
const exitAfter = 1250 * time.Millisecond

type palette struct {
	base, primary, accent, text, dim, bad, good, warn rgba
}

// look is what a drawn frame asks of the background and the last pass.
type look struct {
	accent rgba
	energy float32
	fade   float32
	glitch float32
}

// scene lays the login screen out and draws it. Sizes are written for a
// 1920x1080 screen and scaled to whatever is there.
type scene struct {
	cfg  settings.Config
	pal  palette
	txt  *text
	b    batch
	host string

	w, h  float32
	scale float32

	// Values that glide towards where the state says they should be.
	frame     [4]float32 // the focus frame: centre x, centre y, half width, half height
	frameSet  bool
	zoneLit   [4]float32
	fprintMix float32 // 0 password, 1 fingerprint
	checkMix  float32
	insertMix float32
}

func newScene(cfg settings.Config) (*scene, error) {
	txt, err := newText()
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	return &scene{
		cfg: cfg,
		txt: txt,
		pal: palette{
			base:    fromConfig(cfg.Base),
			primary: fromConfig(cfg.Primary),
			accent:  fromConfig(cfg.Accent),
			text:    fromConfig(cfg.Text),
			dim:     fromConfig(cfg.Dim),
			bad:     fromConfig(cfg.Error),
			good:    fromConfig(cfg.Success),
			warn:    fromConfig(cfg.Warning),
		},
		host: host,
	}, nil
}

// since is the seconds from at to now, or a long time if at never happened.
func since(now, at time.Time) float32 {
	if at.IsZero() {
		return 1e6
	}
	return float32(now.Sub(at).Seconds())
}

// approach moves v towards target, covering most of the distance in about
// 1/speed seconds.
func approach(v *float32, target, dt, speed float32) {
	*v += (target - *v) * (1 - exp(-dt*speed))
}

// flag is 1 for true and 0 for false.
func flag(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// draw fills the batch with the screen for st. age is the seconds since the
// greeter appeared and t a clock for anything that moves.
func (sc *scene) draw(st *greeter.State, now time.Time, w, h int, age, t, dt float32) look {
	sc.b.reset()
	sc.w, sc.h = float32(w), float32(h)
	sc.scale = max(min(sc.w/1920, sc.h/1080), 0.35)

	b, txt, pal := &sc.b, sc.txt, sc.pal
	u := func(v float32) float32 { return v * sc.scale }
	Y := func(y float32) float32 { return sc.h/2 + (y-540)*sc.scale }
	// in fades an element in, a little after the one before it.
	in := func(delay float32) float32 { return ease((age - delay) / 0.7) }

	sinceFail := since(now, st.FailedAt)
	sinceOK := since(now, st.AcceptedAt)
	welcome := st.Welcome()

	// Everything lit takes the colour of the moment. A rejected login
	// holds red for a moment, then lets go quickly: halfway between the
	// two colours is just grey.
	fail := smooth((1.5 - sinceFail) / 0.45)
	if st.SessionFailed {
		fail = 1
	}
	jolt := exp(-sinceFail * 2.5) // the hit itself, which always fades
	okMix := float32(0)
	if welcome {
		okMix = smooth(sinceOK / 0.25)
	}
	accent := mix(mix(pal.accent, pal.bad, fail), pal.good, okMix)
	typed := exp(-since(now, st.TypedAt) * 7)

	approach(&sc.fprintMix, flag(st.Fprint), dt, 10)
	approach(&sc.checkMix, flag(st.Checking), dt, 12)
	approach(&sc.insertMix, flag(st.Insert), dt, 16)
	for z := range sc.zoneLit {
		lit := st.Focus == greeter.Zone(z) && !st.LoggedIn
		approach(&sc.zoneLit[z], flag(lit), dt, 14)
	}
	zone := func(z greeter.Zone) float32 { return lerp(0.72, 1, sc.zoneLit[z]) }

	// A rejected login shakes the column.
	cx := sc.w / 2
	if sinceFail < 1 && !st.SessionFailed {
		cx += sin(sinceFail*46) * u(15) * exp(-sinceFail*7)
	}

	sc.corners(st, now, t, in(0.1), accent, fail)

	// ---- The ring ----
	{
		a := in(0.15)
		ry := Y(292) + (1-a)*u(-16)
		R := u(118)
		pw := 1 - sc.fprintMix

		b.ring(cx, ry, R+u(4), 0, 0, pal.base.alpha(0.6*a))
		b.ring(cx, ry, R, u(2.5)+typed*u(2.5), u(9)+typed*u(9), accent.alpha(a))
		b.ring(cx, ry, R+u(36), u(1), 0, pal.dim.alpha(0.22*a))
		for i := range 4 {
			b.arc(cx, ry, R+u(36), u(2), 0, -t*0.22+float32(i)*tau/4, 0.42, pal.dim.alpha(0.9*a))
		}

		// Password: three arcs circling. Fingerprint: one segment per try.
		for i := range 3 {
			b.arc(cx, ry, R+u(19), u(2), u(3), t*0.4+float32(i)*tau/3, 1.3, accent.alpha(0.85*a*pw))
		}
		if n := st.FprintMaxTries; n > 0 && sc.fprintMix > 0.01 {
			const gap = 0.22
			used := n - max(st.FprintTries, 0)
			sweep := float32(tau)/float32(n) - gap
			for i := range n {
				c := accent
				if i < used {
					c = pal.bad
				}
				start := -float32(tau)/4 + gap/2 + float32(i)*float32(tau)/float32(n)
				b.arc(cx, ry, R+u(19), u(3), u(3), start, sweep, c.alpha(0.9*a*sc.fprintMix))
			}
		}

		// Password: the user's initial in a hexagon.
		if pw > 0.01 {
			b.hex(cx, ry, u(66), 0, 0, accent.alpha(0.07*a*pw))
			b.hex(cx, ry, u(66), u(2), u(5), accent.alpha(0.9*a*pw))
			initial := "?"
			for _, r := range strings.ToUpper(st.User()) {
				initial = string(r)
				break
			}
			txt.centred(b, fontDisplayBold, u(60), cx, ry, pal.text.alpha(a*pw), initial, 0)
		}

		// Fingerprint: ridges, and a bar sweeping over them while reading.
		if fp := sc.fprintMix; fp > 0.01 {
			for k := range 6 {
				fk := float32(k)
				gap := 1.3 + 0.7*sin(fk*1.9+1)
				alpha := float32(0.85)
				if st.Scanning {
					alpha = 0.5 + 0.5*sin(t*11-fk*0.9)
				}
				start := float32(tau)/4 + gap/2 + 0.35*sin(fk*2.7)
				b.arc(cx, ry, u(18)+fk*u(13), u(3), u(2), start, float32(tau)-gap, accent.alpha(alpha*a*fp))
			}
			if st.Scanning {
				dy := sin(t*3.4) * u(84)
				half := sqrt(max(u(92)*u(92)-dy*dy, 0))
				b.box(cx, ry+dy, half, u(1.5), 0, 0, u(7), accent.alpha(a*fp))
			}
		}

		if sc.checkMix > 0.01 {
			b.arc(cx, ry, R+u(9), u(3), u(6), t*7.5, 1.5, pal.text.alpha(a*sc.checkMix))
		}

		// Accepted: rings run out across the screen.
		if welcome {
			for i := range 3 {
				p := (sinceOK - float32(i)*0.16) / 0.95
				if p > 0 && p < 1 {
					b.ring(cx, ry, R+u(1100)*ease(p), u(3), u(10), pal.good.alpha(1-p))
				}
			}
		}
	}

	// ---- The user ----
	name := strings.ToUpper(st.User())
	nameW := txt.width(fontDisplay, u(44), name, u(7))
	{
		a := in(0.25) * zone(greeter.ZoneUser)
		y := Y(500)
		slide := ease(since(now, st.UserChangedAt) / 0.24)
		x := cx + float32(st.UserDir)*u(40)*(1-slide)
		txt.centred(b, fontDisplay, u(44), x, y, pal.text.alpha(a*slide), name, u(7))

		if len(st.Users) > 1 {
			c := mix(pal.dim, accent, sc.zoneLit[greeter.ZoneUser]).alpha(a)
			off := max(nameW, u(170))/2 + u(46)
			txt.centred(b, fontMono, u(30), cx-off, y, c, "‹", 0)
			txt.centred(b, fontMono, u(30), cx+off, y, c, "›", 0)
		}
		if n := len(st.Users); n > 1 && n <= 16 {
			x0 := cx - float32(n-1)*u(18)/2
			for i := range n {
				if i == st.UserIdx {
					b.hex(x0+float32(i)*u(18), Y(548), u(4.5), 0, u(3), accent.alpha(a))
				} else {
					b.hex(x0+float32(i)*u(18), Y(548), u(3.5), 0, 0, pal.dim.alpha(0.6*a))
				}
			}
		}
	}

	// ---- The login field ----
	{
		a := in(0.35) * zone(greeter.ZoneAuth)
		y := Y(640)
		hw, hh := u(260), u(31)
		lit := sc.zoneLit[greeter.ZoneAuth]
		edge := mix(pal.dim.alpha(0.7), accent, max(lit, fail))

		b.box(cx, y, hw, hh, u(12), 0, 0, pal.base.alpha(0.78*a))
		b.box(cx, y, hw, hh, u(12), u(1.5), u(4)*lit+u(5)*sc.insertMix, edge.alpha(a))

		pw := 1 - sc.fprintMix
		if n := st.PasswordLen(); n > 0 {
			step := min(u(24), u(440)/float32(n))
			x0 := cx - float32(n-1)*step/2
			for i := range n {
				r, glow := u(5.5), float32(0)
				if i == n-1 {
					r *= 1 + typed*0.9
					glow = typed * u(5)
				}
				b.hex(x0+float32(i)*step, y, r, 0, glow, pal.text.alpha(a*pw))
			}
			if st.Insert {
				blink := 0.35 + 0.65*flag(math.Mod(float64(t), 1) < 0.55)
				b.box(x0+float32(n-1)*step+u(17), y, u(1.2), u(11), 0, 0, u(3), accent.alpha(a*pw*blink))
			}
		} else if !st.LoggedIn {
			hint := "PRESS  I  TO TYPE"
			if st.Insert {
				hint = "ENTER PASSWORD"
			}
			hintW := txt.centred(b, fontMono, u(14), cx, y, pal.dim.alpha(a*pw), hint, u(3.5))
			if st.Insert {
				blink := 0.35 + 0.65*flag(math.Mod(float64(t), 1) < 0.55)
				b.box(cx-hintW/2-u(16), y, u(1.2), u(11), 0, 0, u(3), accent.alpha(a*pw*blink))
			}
		}
		if fp := sc.fprintMix; fp > 0.01 {
			label, c := "TOUCH THE READER", pal.text
			switch {
			case st.FprintFail:
				label, c = "NO MATCH", pal.bad
			case st.Scanning:
				label, c = "READING", accent
			}
			txt.centred(b, fontMono, u(14), cx, y, c.alpha(a*fp), label, u(3.5))
		}
		if sc.checkMix > 0.01 {
			b.box(cx+sin(t*4.5)*(hw-u(60)), y+hh-u(1), u(42), u(1.5), 0, 0, u(6), accent.alpha(a*sc.checkMix))
		}
	}

	// ---- What just happened ----
	if msg, bad := st.Status(); msg != "" {
		c := pal.text
		switch {
		case welcome:
			c = pal.good
		case st.ErrMsg != "":
			c = pal.bad
		case bad:
			c = pal.warn
		}
		msg = strings.ToUpper(msg)
		msgW := txt.centred(b, fontMono, u(14), cx, Y(704), c.alpha(in(0.35)), msg, u(2.5))
		if welcome || st.Checking {
			dots := strings.Repeat(".", int(t*3)%4)
			txt.draw(b, fontMono, u(14), cx+msgW/2+u(4), Y(704)+capHeight*float32(pixels(u(14)))/2, c, dots, u(2.5))
		}
	}

	// ---- Session ----
	{
		a := in(0.45) * zone(greeter.ZoneSession)
		lit := sc.zoneLit[greeter.ZoneSession]
		txt.centred(b, fontMono, u(11), cx, Y(790), pal.dim.alpha(a), "SESSION", u(4.5))
		session := strings.ToUpper(st.Session())
		sw := txt.centred(b, fontDisplay, u(22), cx, Y(824), pal.text.alpha(a), session, u(4))
		if len(st.Sessions) > 1 {
			c := mix(pal.dim, accent, lit).alpha(a)
			off := max(sw, u(120))/2 + u(34)
			txt.centred(b, fontMono, u(22), cx-off, Y(824), c, "‹", 0)
			txt.centred(b, fontMono, u(22), cx+off, Y(824), c, "›", 0)
		}
	}

	// ---- Power ----
	{
		a := in(0.55) * zone(greeter.ZonePower)
		lit := sc.zoneLit[greeter.ZonePower]
		txt.centred(b, fontMono, u(11), cx, Y(880), pal.dim.alpha(a), "POWER", u(4.5))
		for i, label := range []string{"SHUT DOWN", "REBOOT"} {
			x := cx + (float32(i)*2-1)*u(92)
			y := Y(920)
			selected := i == st.PowerIdx
			edge, ink := pal.dim.alpha(0.4), pal.dim
			if selected {
				edge, ink = mix(pal.dim, accent, lit), pal.text
			}
			glow := float32(0)
			if selected && st.PowerArmed {
				throb := 0.5 + 0.5*sin(t*10)
				b.box(x, y, u(84), u(19), u(8), 0, 0, pal.bad.alpha((0.2+0.25*throb)*a))
				edge, glow = pal.bad, u(6)
			} else if selected {
				glow = u(3) * lit
			}
			b.box(x, y, u(84), u(19), u(8), u(1.5), glow, edge.alpha(a))
			txt.centred(b, fontMono, u(13), x, y, ink.alpha(a), label, u(2.5))
		}
	}

	// ---- The focus frame ----
	{
		var target [4]float32
		switch st.Focus {
		case greeter.ZoneUser:
			target = [4]float32{cx, Y(516), max(nameW/2+u(96), u(236)), u(56)}
		case greeter.ZoneAuth:
			target = [4]float32{cx, Y(640), u(286), u(50)}
		case greeter.ZoneSession:
			target = [4]float32{cx, Y(812), u(236), u(44)}
		case greeter.ZonePower:
			target = [4]float32{cx, Y(908), u(214), u(50)}
		}
		if !sc.frameSet {
			sc.frame, sc.frameSet = target, true
		}
		for i := range sc.frame {
			approach(&sc.frame[i], target[i], dt, 16)
		}
		a := in(0.6) * (1 - okMix)
		if st.LoggedIn {
			a = 0
		}
		fx, fy, fw, fh := sc.frame[0], sc.frame[1], sc.frame[2], sc.frame[3]
		arm, thick := u(16), u(2)
		for _, sx := range []float32{-1, 1} {
			for _, sy := range []float32{-1, 1} {
				px, py := fx+sx*fw, fy+sy*fh
				b.box(px-sx*arm/2, py, arm/2, thick/2, 0, 0, u(3), accent.alpha(a))
				b.box(px, py-sy*arm/2, thick/2, arm/2, 0, 0, u(3), accent.alpha(a))
			}
		}
	}

	// ---- Keys ----
	txt.centred(b, fontMono, u(13), sc.w/2, sc.h-u(58), pal.dim.alpha(in(0.7)), st.Hint(), u(0.5))

	lk := look{accent: accent, fade: smooth(age / 0.9)}
	lk.energy = max(typed*0.35, jolt, okMix)
	if !st.SessionFailed {
		lk.glitch = clamp01(1 - sinceFail/0.4)
	}
	if welcome {
		lk.fade *= 1 - smooth((sinceOK-0.8)/0.4)
	}
	return lk
}

// corners draws what sits around the edge of the screen: the name, the
// clock, the mode and what the greeter is waiting for.
func (sc *scene) corners(st *greeter.State, now time.Time, t, a float32, accent rgba, fail float32) {
	b, txt, pal := &sc.b, sc.txt, sc.pal
	u := func(v float32) float32 { return v * sc.scale }
	m := u(56)

	// Brackets in the corners of the screen.
	arm, inset := u(26), u(26)
	for _, sx := range []float32{-1, 1} {
		for _, sy := range []float32{-1, 1} {
			px := sc.w/2 + sx*(sc.w/2-inset)
			py := sc.h/2 + sy*(sc.h/2-inset)
			b.box(px-sx*arm/2, py, arm/2, u(0.75), 0, 0, 0, pal.dim.alpha(0.7*a))
			b.box(px, py-sy*arm/2, u(0.75), arm/2, 0, 0, 0, pal.dim.alpha(0.7*a))
		}
	}

	// Top left: the name, and the same thing in hex.
	hexW := txt.width(fontDisplayBold, u(26), "HEX", u(8))
	txt.draw(b, fontDisplayBold, u(26), m, m+u(24), pal.text.alpha(a), "HEX", u(8))
	txt.draw(b, fontDisplayBold, u(26), m+hexW+u(16), m+u(24), accent.alpha(a), "DM", u(8))
	txt.draw(b, fontMono, u(11), m+u(1), m+u(46), pal.dim.alpha(a), "48 45 58 20 44 4D", u(2))

	// Top right: the clock.
	clock, tail := now.Format("15:04"), now.Format("05")
	if !sc.cfg.Clock24 {
		clock, tail = now.Format("03:04"), now.Format("05 PM")
	}
	tailW := txt.right(b, fontMono, u(16), sc.w-m, m+u(32), accent.alpha(a), tail, u(1))
	txt.right(b, fontDisplay, u(40), sc.w-m-tailW-u(10), m+u(32), pal.text.alpha(a), clock, u(3))
	txt.right(b, fontMono, u(12), sc.w-m, m+u(56), pal.dim.alpha(a), strings.ToUpper(now.Format("Mon 02 Jan 2006")), u(3))

	// Bottom left: the mode, as an editor would show it, and the machine.
	mode := "NORMAL"
	if st.Insert {
		mode = "INSERT"
	}
	chipW := u(86)
	chipX, chipY := m+chipW/2, sc.h-m-u(4)
	if st.Insert {
		b.box(chipX, chipY, chipW/2, u(13), u(5), 0, u(4), accent.alpha(a))
		txt.centred(b, fontMono, u(12), chipX, chipY, pal.base.alpha(a), mode, u(2.5))
	} else {
		b.box(chipX, chipY, chipW/2, u(13), u(5), u(1.2), 0, pal.dim.alpha(a))
		txt.centred(b, fontMono, u(12), chipX, chipY, pal.dim.alpha(a), mode, u(2.5))
	}
	if sc.host != "" {
		txt.draw(b, fontMono, u(12), m+chipW+u(16), chipY+capHeight*float32(pixels(u(12)))/2, pal.dim.alpha(a), strings.ToUpper(sc.host), u(2.5))
	}

	// Bottom right: what the greeter is doing.
	label, c := "AWAITING LOGIN", pal.dim
	switch {
	case st.Welcome():
		label, c = "ACCESS GRANTED", pal.good
	case st.SessionFailed:
		label, c = "SESSION FAILED", pal.bad
	case fail > 0.25:
		label, c = "ACCESS DENIED", pal.bad
	case st.Checking:
		label, c = "AUTHENTICATING", accent
	case st.Scanning:
		label, c = "READING FINGERPRINT", accent
	case st.Fprint:
		label, c = "FINGERPRINT READY", accent
	}
	labelW := txt.right(b, fontMono, u(12), sc.w-m, chipY+capHeight*float32(pixels(u(12)))/2, c.alpha(a), label, u(3))
	beat := 0.55 + 0.45*sin(t*3)
	b.ring(sc.w-m-labelW-u(14), chipY, u(3.5), 0, u(4), c.alpha(a*beat))
}
