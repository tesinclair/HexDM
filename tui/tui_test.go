package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// testModel returns a model focussed on the password box in insert mode,
// with buffered channels so nothing in Update can block.
func testModel(t *testing.T) Model {
	t.Helper()
	m := New()
	m.users = []string{"ada", "bob"}
	m.sessions = []string{"Sway", "Hyprland"}
	m.user = make(chan string, 8)
	m.password = make(chan []byte, 8)
	m.session = make(chan string, 8)
	m.powerReq = make(chan string, 8)
	m.out = make(chan func(), 8)
	m.fprintdTries = -1
	m.componentIdx = 1
	m.focussed = "login"

	normalMode = false
	t.Cleanup(func() { normalMode = true })
	return *m
}

func press(m Model, keys ...tea.KeyMsg) Model {
	for _, k := range keys {
		next, _ := m.Update(k)
		m = next.(Model)
	}
	return m
}

// deliver performs the sends Update queued for the main loop.
func deliver(t *testing.T, m Model) {
	t.Helper()
	if len(m.out) == 0 {
		t.Fatal("nothing was queued for main")
	}
	for len(m.out) > 0 {
		(<-m.out)()
	}
}

func typed(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestPasswordOnlyTakesCharacters(t *testing.T) {
	m := press(testModel(t),
		typed("a"),
		tea.KeyMsg{Type: tea.KeyLeft},
		tea.KeyMsg{Type: tea.KeyTab},
		tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}},
		tea.KeyMsg{Type: tea.KeyCtrlA},
		typed("b"),
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pasted"), Paste: true},
	)
	if got := string(m.passwordText); got != "a bpasted" {
		t.Errorf("password = %q, want %q", got, "a bpasted")
	}
}

func TestPasswordBackspaceRemovesWholeCharacter(t *testing.T) {
	m := press(testModel(t), typed("aé"), tea.KeyMsg{Type: tea.KeyBackspace})
	if got := string(m.passwordText); got != "a" {
		t.Errorf("password = %q, want %q", got, "a")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyBackspace})
	if got := string(m.passwordText); got != "" {
		t.Errorf("password = %q, want empty", got)
	}
}

func TestPasswordIsWipedOnSubmit(t *testing.T) {
	m := press(testModel(t), typed("hunter2"))
	buf := m.passwordText[:cap(m.passwordText)]

	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	deliver(t, m)

	if got := string(<-m.password); got != "hunter2" {
		t.Errorf("sent %q, want %q", got, "hunter2")
	}
	if len(m.passwordText) != 0 {
		t.Errorf("field still holds %d bytes", len(m.passwordText))
	}
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("buffer byte %d not wiped", i)
		}
	}
}

func TestPasswordIsWipedOnUserSwitch(t *testing.T) {
	m := press(testModel(t), typed("hunter2"), tea.KeyMsg{Type: tea.KeyEsc})
	buf := m.passwordText[:cap(m.passwordText)]
	m.componentIdx = 0
	m.focussed = "users"
	m = press(m, typed("l"))
	if len(m.passwordText) != 0 {
		t.Errorf("field still holds %d bytes", len(m.passwordText))
	}
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("buffer byte %d not wiped", i)
		}
	}
}

func TestEventsReachMainInOrder(t *testing.T) {
	m := testModel(t)
	normalMode = true
	m.componentIdx = 0
	m.focussed = "users"
	m = press(m, typed("l"), typed("l"), typed("h"))
	deliver(t, m)
	close(m.user)
	var got []string
	for u := range m.user {
		got = append(got, u)
	}
	if len(got) != 3 || got[0] != "bob" || got[1] != "ada" || got[2] != "bob" {
		t.Errorf("user changes arrived as %q", got)
	}
}

func TestLoginAcceptedQuietsTheUI(t *testing.T) {
	m := press(testModel(t), typed("secret"))
	buf := m.passwordText[:cap(m.passwordText)]

	next, _ := m.Update(newAuthMsg(LoginAccepted))
	m = next.(Model)
	if !m.loggedIn {
		t.Fatal("expected loggedIn")
	}
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("password byte %d not wiped", i)
		}
	}

	// Keys are ignored and the ticker is not re-armed.
	m = press(m, typed("x"), tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.passwordText) != 0 || len(m.out) != 0 {
		t.Error("input was still processed after login")
	}
	if _, cmd := m.Update(tickMsg(true)); cmd != nil {
		t.Error("ticker still running after login")
	}

	// The session ending still closes the UI.
	if _, cmd := m.Update(newAuthMsg(nil)); cmd == nil {
		t.Error("expected quit when the session ends")
	}
}

func TestEnterSubmitsFromNormalMode(t *testing.T) {
	m := press(testModel(t), typed("hunter2"), tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEnter})
	deliver(t, m)
	if got := string(<-m.password); got != "hunter2" {
		t.Errorf("sent %q, want %q", got, "hunter2")
	}
	if !m.checking {
		t.Error("expected the checking state after submitting")
	}
	next, _ := m.Update(newAuthMsg(ErrFingerprintNoMatch))
	if next.(Model).checking {
		t.Error("checking should clear once main answers")
	}
}

func TestEnterDoesNotSubmitAnEmptyPassword(t *testing.T) {
	m := press(testModel(t), tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.out) != 0 {
		t.Error("an empty password was submitted")
	}
}
