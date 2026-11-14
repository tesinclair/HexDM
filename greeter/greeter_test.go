package greeter

import (
	"testing"
	"time"
)

var t0 = time.Unix(1_700_000_000, 0)

func testLink() *Link {
	return &Link{
		Fprintd:       make(chan bool, 8),
		FprintdTries:  make(chan int, 8),
		ScannerActive: make(chan bool, 8),
		Auth:          make(chan error, 8),
		SessionIdx:    make(chan int, 8),
		Kill:          make(chan bool, 8),
		User:          make(chan string, 8),
		Password:      make(chan []byte, 8),
		Session:       make(chan string, 8),
		Power:         make(chan string, 8),
		Released:      make(chan struct{}, 1),
	}
}

// testState returns a state focussed on the password field in insert mode.
// Sends to main are queued but not delivered until deliver is called.
func testState(t *testing.T) *State {
	t.Helper()
	return &State{
		Users:       []string{"ada", "bob"},
		Sessions:    []string{"Sway", "Hyprland"},
		Focus:       ZoneAuth,
		Insert:      true,
		FprintTries: -1,
		password:    make([]byte, 0, maxPasswordLen),
		link:        testLink(),
		out:         make(chan func(), 8),
	}
}

func press(s *State, keys ...Key) {
	for _, k := range keys {
		s.Key(k, t0)
	}
}

func typed(text string) Key { return Key{Type: KeyText, Text: text} }

// deliver performs the sends queued for the main loop.
func deliver(t *testing.T, s *State) {
	t.Helper()
	if len(s.out) == 0 {
		t.Fatal("nothing was queued for main")
	}
	for len(s.out) > 0 {
		(<-s.out)()
	}
}

func wiped(t *testing.T, buf []byte) {
	t.Helper()
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("buffer byte %d not wiped", i)
		}
	}
}

func TestPasswordOnlyTakesCharacters(t *testing.T) {
	s := testState(t)
	press(s, typed("a"), Key{Type: KeyLeft}, Key{Type: KeyRight}, typed(" "), typed("b"), typed("pasted"))
	if got := string(s.password); got != "a bpasted" {
		t.Errorf("password = %q, want %q", got, "a bpasted")
	}
}

func TestPasswordBackspaceRemovesWholeCharacter(t *testing.T) {
	s := testState(t)
	press(s, typed("aé"), Key{Type: KeyBackspace})
	if got := string(s.password); got != "a" {
		t.Errorf("password = %q, want %q", got, "a")
	}
	press(s, Key{Type: KeyBackspace}, Key{Type: KeyBackspace})
	if got := string(s.password); got != "" {
		t.Errorf("password = %q, want empty", got)
	}
}

func TestPasswordStopsAtTheBufferSize(t *testing.T) {
	s := testState(t)
	for range maxPasswordLen + 10 {
		press(s, typed("x"))
	}
	if len(s.password) != maxPasswordLen || cap(s.password) != maxPasswordLen {
		t.Errorf("len %d cap %d, want both %d", len(s.password), cap(s.password), maxPasswordLen)
	}
}

func TestPasswordIsWipedOnSubmit(t *testing.T) {
	s := testState(t)
	press(s, typed("hunter2"))
	buf := s.password[:cap(s.password)]

	press(s, Key{Type: KeyEnter})
	deliver(t, s)

	if got := string(<-s.link.Password); got != "hunter2" {
		t.Errorf("sent %q, want %q", got, "hunter2")
	}
	if len(s.password) != 0 {
		t.Errorf("field still holds %d bytes", len(s.password))
	}
	wiped(t, buf)
}

func TestPasswordIsWipedOnUserSwitch(t *testing.T) {
	s := testState(t)
	press(s, typed("hunter2"), Key{Type: KeyEsc})
	buf := s.password[:cap(s.password)]
	press(s, typed("k"), typed("l"))
	if s.Focus != ZoneUser || s.User() != "bob" {
		t.Fatalf("focus %v user %q, want the user row on bob", s.Focus, s.User())
	}
	if len(s.password) != 0 {
		t.Errorf("field still holds %d bytes", len(s.password))
	}
	wiped(t, buf)
}

func TestEventsReachMainInOrder(t *testing.T) {
	s := testState(t)
	press(s, Key{Type: KeyUp}, typed("l"), typed("l"), typed("h"))
	deliver(t, s)
	close(s.link.User)
	var got []string
	for u := range s.link.User {
		got = append(got, u)
	}
	if len(got) != 3 || got[0] != "bob" || got[1] != "ada" || got[2] != "bob" {
		t.Errorf("user changes arrived as %q", got)
	}
}

func TestLoginAcceptedQuietsTheUI(t *testing.T) {
	s := testState(t)
	press(s, typed("secret"))
	buf := s.password[:cap(s.password)]

	s.Handle(AuthEvent{LoginAccepted}, t0)
	if !s.LoggedIn || !s.Welcome() {
		t.Fatal("expected loggedIn")
	}
	wiped(t, buf)

	// Keys are ignored.
	press(s, typed("x"), Key{Type: KeyEnter}, Key{Type: KeyDown})
	if len(s.password) != 0 || len(s.out) != 0 || s.Focus != ZoneAuth {
		t.Error("input was still processed after login")
	}

	// The session ending still closes the UI.
	s.Handle(AuthEvent{nil}, t0)
	if !s.Done || s.ExitErr != nil {
		t.Error("expected a clean stop when the session ends")
	}
}

func TestSessionFailureStaysOnScreen(t *testing.T) {
	s := testState(t)
	s.Handle(AuthEvent{LoginAccepted}, t0)
	s.Handle(AuthEvent{ErrSessionFailed}, t0)
	s.Tick(t0.Add(time.Hour))
	if msg, bad := s.Status(); !bad || msg == "" || s.Welcome() {
		t.Errorf("status = %q (bad %v), want the session error", msg, bad)
	}
}

func TestEnterSubmitsFromNormalMode(t *testing.T) {
	s := testState(t)
	press(s, typed("hunter2"), Key{Type: KeyEsc}, Key{Type: KeyEnter})
	deliver(t, s)
	if got := string(<-s.link.Password); got != "hunter2" {
		t.Errorf("sent %q, want %q", got, "hunter2")
	}
	if !s.Checking {
		t.Error("expected the checking state after submitting")
	}
	s.Handle(AuthEvent{ErrFingerprintNoMatch}, t0)
	if s.Checking {
		t.Error("checking should clear once main answers")
	}
}

func TestEnterDoesNotSubmitAnEmptyPassword(t *testing.T) {
	s := testState(t)
	press(s, Key{Type: KeyEnter}, Key{Type: KeyEsc}, Key{Type: KeyEnter})
	if len(s.out) != 0 {
		t.Error("an empty password was submitted")
	}
}

func TestErrorsExpire(t *testing.T) {
	s := testState(t)
	s.FprintTries = 2
	s.Handle(AuthEvent{ErrFingerprintNoMatch}, t0)
	if s.ErrMsg == "" || !s.FprintFail {
		t.Fatal("expected an error and the failed fingerprint flag")
	}
	s.Tick(t0.Add(fprintFailShown + time.Millisecond))
	if s.FprintFail || s.ErrMsg == "" {
		t.Error("the fingerprint flag should clear first, the message later")
	}
	s.Tick(t0.Add(errorShown + time.Millisecond))
	if s.ErrMsg != "" {
		t.Error("the message should have expired")
	}
}

func TestFingerprintFallsBackToTyping(t *testing.T) {
	s := testState(t)
	s.Handle(TriesEvent(3), t0)
	s.Handle(FprintEvent(true), t0)
	if !s.Fprint || s.Insert {
		t.Fatal("expected fingerprint mode, not typing")
	}
	press(s, typed("x"))
	if len(s.password) != 0 {
		t.Error("typing reached the password while the reader was active")
	}
	s.Handle(TriesEvent(0), t0)
	if s.Fprint || !s.Insert {
		t.Error("out of tries should land on the password field, typing")
	}
}

func TestFingerprintToggle(t *testing.T) {
	s := testState(t)
	press(s, Key{Type: KeyEsc}, typed("h"))
	if s.Fprint || s.Warning == "" {
		t.Error("no fingerprint on offer: expected a warning and no change")
	}
	s.FprintTries = 3
	press(s, typed("h"))
	if !s.Fprint {
		t.Error("expected fingerprint mode")
	}
}

func TestSessionChangeIsSent(t *testing.T) {
	s := testState(t)
	press(s, Key{Type: KeyDown}, typed("l"))
	deliver(t, s)
	if got := <-s.link.Session; got != "Hyprland" {
		t.Errorf("sent %q, want Hyprland", got)
	}
	s.Handle(SessionEvent(0), t0)
	s.Handle(SessionEvent(7), t0)
	if s.Session() != "Sway" {
		t.Errorf("session = %q, want Sway", s.Session())
	}
}

func TestPowerNeedsConfirming(t *testing.T) {
	s := testState(t)
	press(s, Key{Type: KeyUp}, Key{Type: KeyUp}) // wraps to the power row
	if s.Focus != ZonePower {
		t.Fatalf("focus = %v, want power", s.Focus)
	}
	press(s, Key{Type: KeyEnter})
	if !s.PowerArmed || len(s.out) != 0 {
		t.Fatal("first enter should only arm")
	}

	// Any other key, or waiting, disarms.
	press(s, typed("l"))
	if s.PowerArmed || s.PowerIdx != 1 {
		t.Fatal("moving should disarm and select reboot")
	}
	press(s, Key{Type: KeyEnter})
	s.Tick(t0.Add(powerArmedFor + time.Millisecond))
	if s.PowerArmed {
		t.Fatal("arming should time out")
	}

	press(s, Key{Type: KeyEnter}, Key{Type: KeyEnter})
	deliver(t, s)
	if got := <-s.link.Power; got != PowerReboot {
		t.Errorf("sent %q, want %q", got, PowerReboot)
	}
}

func TestQuitOnlyWhenAllowed(t *testing.T) {
	s := testState(t)
	press(s, Key{Type: KeyInterrupt}, Key{Type: KeyEsc}, typed("q"))
	if s.Done {
		t.Fatal("quit without AllowQuit")
	}
	s.AllowQuit = true
	press(s, typed("q"))
	if !s.Done || s.ExitErr != UserQuit {
		t.Error("expected UserQuit")
	}
}
