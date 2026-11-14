// Package greeter holds the login screen's state and behaviour: what is
// selected, what the keys do, and what is sent to and received from the main
// loop. It knows nothing about how the screen is drawn. The rendered greeter
// (package gfx) and the terminal fallback (package tui) are both views of
// the same State.
package greeter

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"time"
	"unicode/utf8"
)

var (
	UserQuit              = errors.New("User Quit Session")
	ErrSessionFailed      = errors.New("Failed To Launch Your Session.")
	ErrFingerprintNoMatch = errors.New("Couldn't Match Fingerprint")

	// Not a failure: main sends this on the auth channel when a login has
	// been accepted and the session is about to start. The greeter lets go
	// of the screen and tells main on Link.Released.
	LoginAccepted = errors.New("Login accepted")
)

const (
	PowerShutdown = "shutdown"
	PowerReboot   = "reboot"
)

// Longest password the field accepts, in bytes.
const maxPasswordLen = 512

const (
	errorShown      = 3 * time.Second
	warningShown    = 2 * time.Second
	fprintFailShown = 1 * time.Second
	powerArmedFor   = 4 * time.Second
)

// Zone is one of the rows of the screen that can have focus.
type Zone int

const (
	ZoneUser Zone = iota
	ZoneAuth
	ZoneSession
	ZonePower
	zoneCount
)

type KeyType int

const (
	KeyText KeyType = iota // typed or pasted characters, in Key.Text
	KeyEnter
	KeyEsc
	KeyBackspace
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyInterrupt // ctrl+c
)

type Key struct {
	Type KeyType
	Text string
}

// Link is every channel between the greeter and the main loop.
type Link struct {
	// From main.
	Fprintd       chan bool  // whether fingerprint login is on offer
	FprintdTries  chan int   // fingerprint tries left, -1 for none
	ScannerActive chan bool  // the reader is looking at a finger right now
	Auth          chan error // result of a login attempt, see State.Handle
	SessionIdx    chan int   // session main wants selected
	Kill          chan bool  // shut the greeter down

	// To main.
	User     chan string
	Password chan []byte
	Session  chan string
	Power    chan string

	// Released is sent once, after a login was accepted and the greeter has
	// let go of the display and the keyboard. Main waits for it before it
	// starts the session.
	Released chan struct{}
}

// Release tells main the screen is free. It never blocks.
func (l *Link) Release() {
	select {
	case l.Released <- struct{}{}:
	default:
	}
}

// Events from main, in the form State.Handle takes them.
type (
	Event        any
	AuthEvent    struct{ Err error }
	FprintEvent  bool
	TriesEvent   int
	ScannerEvent bool
	SessionEvent int
	KillEvent    struct{}
)

// Events starts one permanent reader per channel coming from main and
// returns everything they receive as a single stream, so a send from main is
// always picked up whatever state the screen is in.
func (l *Link) Events() <-chan Event {
	out := make(chan Event, 64)
	go func() {
		for err := range l.Auth {
			out <- AuthEvent{err}
		}
	}()
	go func() {
		for v := range l.Fprintd {
			out <- FprintEvent(v)
		}
	}()
	go func() {
		for v := range l.FprintdTries {
			out <- TriesEvent(v)
		}
	}()
	go func() {
		for v := range l.ScannerActive {
			out <- ScannerEvent(v)
		}
	}()
	go func() {
		for v := range l.SessionIdx {
			out <- SessionEvent(v)
		}
	}()
	go func() {
		for range l.Kill {
			out <- KillEvent{}
		}
	}()
	return out
}

// State is the whole login screen. It is owned by one goroutine (the
// frontend's), which feeds it keys, events from main and the time.
type State struct {
	Users   []string
	UserIdx int

	Sessions   []string
	SessionIdx int

	Focus  Zone
	Insert bool // typing goes into the password field

	PowerIdx   int  // 0 shutdown, 1 reboot
	PowerArmed bool // enter was pressed once; again confirms

	// The password is kept in one fixed buffer that is never reallocated,
	// so it can be wiped.
	password []byte

	Fprint         bool // fingerprint is the login method on screen
	FprintTries    int  // -1 means unavailable
	FprintMaxTries int
	Scanning       bool
	FprintFail     bool // a fingerprint was just rejected

	ErrMsg  string
	Warning string

	Checking      bool // a password has been sent to main and not answered yet
	LoggedIn      bool // a login was accepted; the session is starting or running
	SessionFailed bool

	Done    bool  // the frontend should stop
	ExitErr error // what it should return to main

	AllowQuit bool // q and ctrl+c close the greeter (debugging only)

	// When things last happened, for the views to animate from.
	TypedAt       time.Time
	FailedAt      time.Time
	AcceptedAt    time.Time
	UserChangedAt time.Time
	UserDir       int // -1 or +1: which way the user list last moved

	errUntil        time.Time
	warnUntil       time.Time
	fprintFailUntil time.Time
	powerArmedUntil time.Time

	link *Link
	out  chan func() // sends to main, delivered one at a time in order
}

// New builds the state for the given users and sessions. It waits for the
// first fingerprint values from main, which main queues before starting the
// greeter.
func New(users, sessions []string, link *Link) (*State, error) {
	if len(sessions) == 0 || len(users) == 0 {
		return nil, errors.New("the greeter needs at least one user and one session")
	}
	s := &State{
		Users:     users,
		Sessions:  sessions,
		Focus:     ZoneAuth,
		password:  make([]byte, 0, maxPasswordLen),
		AllowQuit: os.Getenv("HEX_DEBUG") == "1",
		link:      link,
		out:       make(chan func(), 256),
	}
	s.FprintTries = <-link.FprintdTries
	s.FprintMaxTries = s.FprintTries
	s.setFprint(<-link.Fprintd)

	go func() {
		for send := range s.out {
			send()
		}
	}()
	return s, nil
}

// wipe zeroes the whole password buffer and returns it empty.
func wipe(buf []byte) []byte {
	buf = buf[:cap(buf)]
	clear(buf)
	return buf[:0]
}

// PasswordLen is the number of characters typed so far.
func (s *State) PasswordLen() int { return utf8.RuneCount(s.password) }

// User is the selected user's name.
func (s *State) User() string { return s.Users[s.UserIdx] }

// Session is the selected session's name.
func (s *State) Session() string { return s.Sessions[s.SessionIdx] }

// toMain queues a send to the main loop. A single goroutine performs the
// queued sends one after another, so main sees a user change, a session
// choice and a password in the order they were made, and the frontend never
// blocks on a busy main loop. It reports false if the queue is full and the
// send was dropped.
func (s *State) toMain(send func()) bool {
	select {
	case s.out <- send:
		return true
	default:
		log.Println("Main loop is not keeping up. Dropping input.")
		return false
	}
}

// setFprint switches between the fingerprint reader and the password field.
// Landing on the password field with focus there goes straight to typing.
func (s *State) setFprint(active bool) {
	s.Fprint = active
	s.Insert = !active && s.Focus == ZoneAuth
}

// submitPassword queues the typed password for main and wipes the field. It
// does nothing in fingerprint mode or with an empty field.
func (s *State) submitPassword() {
	if s.Fprint || len(s.password) == 0 {
		return
	}
	// Hand main its own copy and wipe ours straight away. Main wipes its
	// copy once PAM has seen it.
	pass := bytes.Clone(s.password)
	s.password = wipe(s.password)
	link := s.link
	if s.toMain(func() { link.Password <- pass }) {
		s.Checking = true
	} else {
		clear(pass)
	}
}

func (s *State) warn(msg string, now time.Time) {
	s.Warning = msg
	s.warnUntil = now.Add(warningShown)
}

// Key applies one key press.
func (s *State) Key(k Key, now time.Time) {
	if s.LoggedIn || s.Done {
		// Nothing typed from here on is meant for us.
		return
	}
	if k.Type != KeyEnter {
		s.PowerArmed = false
	}

	switch k.Type {
	case KeyInterrupt:
		s.quit()
	case KeyUp:
		s.move(-1)
	case KeyDown:
		s.move(1)
	case KeyLeft:
		if !s.Insert {
			s.change(-1, now)
		}
	case KeyRight:
		if !s.Insert {
			s.change(1, now)
		}
	case KeyEsc:
		s.Insert = false
	case KeyEnter:
		s.enter(now)
	case KeyBackspace:
		if s.Insert {
			// Remove one whole character, not one byte.
			_, size := utf8.DecodeLastRune(s.password)
			keep := len(s.password) - size
			clear(s.password[keep:])
			s.password = s.password[:keep]
			s.TypedAt = now
		}
	case KeyText:
		if s.Insert {
			if len(k.Text) > 0 && len(s.password)+len(k.Text) <= cap(s.password) {
				s.password = append(s.password, k.Text...)
				s.TypedAt = now
			}
			return
		}
		switch k.Text {
		case "q":
			s.quit()
		case "h":
			s.change(-1, now)
		case "l":
			s.change(1, now)
		case "k":
			s.move(-1)
		case "j":
			s.move(1)
		case "i", "a":
			if s.Focus == ZoneAuth && !s.Fprint {
				s.Insert = true
			}
		}
	}
}

func (s *State) quit() {
	if s.AllowQuit {
		s.Done = true
		s.ExitErr = UserQuit
	}
}

// move shifts focus up or down a row.
func (s *State) move(d int) {
	s.Focus = Zone((int(s.Focus) + d + int(zoneCount)) % int(zoneCount))
	s.Insert = false
}

// change is left/right: it changes the value of the focussed row.
func (s *State) change(d int, now time.Time) {
	link := s.link
	switch s.Focus {
	case ZoneUser:
		if len(s.Users) < 2 {
			return
		}
		s.UserIdx = (s.UserIdx + d + len(s.Users)) % len(s.Users)
		s.UserDir = d
		s.UserChangedAt = now
		s.password = wipe(s.password)
		user := s.User()
		s.toMain(func() { link.User <- user })
	case ZoneAuth:
		if s.FprintTries > 0 {
			s.setFprint(!s.Fprint)
		} else {
			s.warn("Fingerprint not available for this user", now)
		}
	case ZoneSession:
		if len(s.Sessions) < 2 {
			return
		}
		s.SessionIdx = (s.SessionIdx + d + len(s.Sessions)) % len(s.Sessions)
		chosen := s.Session()
		s.toMain(func() { link.Session <- chosen })
	case ZonePower:
		s.PowerIdx = 1 - s.PowerIdx
	}
}

func (s *State) enter(now time.Time) {
	switch s.Focus {
	case ZoneAuth:
		// No need to go back into insert mode just to submit.
		s.submitPassword()
	case ZoneUser, ZoneSession:
		// Chosen: on to logging in.
		s.Focus = ZoneAuth
		s.Insert = !s.Fprint
	case ZonePower:
		if !s.PowerArmed {
			s.PowerArmed = true
			s.powerArmedUntil = now.Add(powerArmedFor)
			return
		}
		s.PowerArmed = false
		req := PowerShutdown
		if s.PowerIdx == 1 {
			req = PowerReboot
		}
		link := s.link
		s.toMain(func() { link.Power <- req })
	}
}

// Handle applies one event from main.
func (s *State) Handle(ev Event, now time.Time) {
	switch ev := ev.(type) {
	case KillEvent:
		s.Done = true
	case SessionEvent:
		if idx := int(ev); idx >= 0 && idx < len(s.Sessions) {
			s.SessionIdx = idx
		}
	case ScannerEvent:
		s.Scanning = bool(ev)
	case TriesEvent:
		s.FprintTries = int(ev)
		if s.FprintTries > s.FprintMaxTries {
			s.FprintMaxTries = s.FprintTries
		}
		if s.FprintTries < 1 && s.Fprint {
			// Out of tries: fall back to the password field.
			s.setFprint(false)
		}
	case FprintEvent:
		if !ev {
			s.FprintTries = -1
		}
		s.setFprint(bool(ev))
		s.Warning = ""
		if !s.SessionFailed {
			s.ErrMsg = ""
		}
	case AuthEvent:
		s.Checking = false // main has answered
		switch {
		case ev.Err == nil:
			// The session is over.
			s.Done = true
			s.ExitErr = nil
		case errors.Is(ev.Err, LoginAccepted):
			s.LoggedIn = true
			s.AcceptedAt = now
			s.Insert = false
			s.ErrMsg = ""
			s.Warning = ""
			s.password = wipe(s.password)
		case errors.Is(ev.Err, ErrSessionFailed):
			// Stays up until main shuts us down.
			s.SessionFailed = true
			s.FailedAt = now
			s.ErrMsg = fmt.Sprintf("Session Error: %s", ev.Err)
			s.errUntil = time.Time{}
		default:
			if errors.Is(ev.Err, ErrFingerprintNoMatch) {
				s.FprintFail = true
				s.fprintFailUntil = now.Add(fprintFailShown)
			}
			s.FailedAt = now
			s.ErrMsg = fmt.Sprintf("Login Failed: %s", ev.Err)
			s.errUntil = now.Add(errorShown)
		}
	}
}

// Tick clears whatever has been on screen long enough.
func (s *State) Tick(now time.Time) {
	if s.ErrMsg != "" && !s.errUntil.IsZero() && now.After(s.errUntil) {
		s.ErrMsg = ""
	}
	if s.Warning != "" && now.After(s.warnUntil) {
		s.Warning = ""
	}
	if s.FprintFail && now.After(s.fprintFailUntil) {
		s.FprintFail = false
	}
	if s.PowerArmed && now.After(s.powerArmedUntil) {
		s.PowerArmed = false
	}
}

// Welcome reports that a login has been accepted and nothing has gone wrong
// since.
func (s *State) Welcome() bool { return s.LoggedIn && !s.SessionFailed }

// PowerLabel names the selected power action.
func (s *State) PowerLabel() string {
	if s.PowerIdx == 1 {
		return "reboot"
	}
	return "shut down"
}

// Status is the one line of feedback under the login field, and whether it
// is bad news.
func (s *State) Status() (msg string, bad bool) {
	switch {
	case s.Welcome():
		return "Access granted. Starting your session", false
	case s.ErrMsg != "":
		return s.ErrMsg, true
	case s.PowerArmed:
		return "Press enter again to " + s.PowerLabel(), true
	case s.Warning != "":
		return s.Warning, true
	case s.Checking:
		return "Verifying", false
	}
	return "", false
}

// Hint is the one-line reminder of what the keys do right now.
func (s *State) Hint() string {
	const sep = "   ·   "
	switch {
	case s.LoggedIn:
		return ""
	case s.Insert:
		return "type your password" + sep + "enter  log in" + sep + "esc  navigate"
	case s.Focus == ZoneUser:
		return "h/l  switch user" + sep + "enter  select" + sep + "j/k  move"
	case s.Focus == ZoneAuth && s.Fprint:
		return "touch the fingerprint reader" + sep + "h/l  use a password" + sep + "j/k  move"
	case s.Focus == ZoneAuth:
		keys := "i  type password" + sep + "enter  log in"
		if s.FprintTries > 0 {
			keys += sep + "h/l  use fingerprint"
		}
		return keys + sep + "j/k  move"
	case s.Focus == ZoneSession:
		return "h/l  change session" + sep + "enter  select" + sep + "j/k  move"
	case s.Focus == ZonePower:
		return "h/l  shut down or reboot" + sep + "enter  confirm" + sep + "j/k  move"
	}
	return ""
}
