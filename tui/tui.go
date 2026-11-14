// Package tui is the terminal greeter. It is the fallback for when the
// rendered greeter (package gfx) can't start, so that a broken graphics
// stack never locks anyone out. It draws the same greeter.State.
package tui

import (
	"log"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	lg "github.com/charmbracelet/lipgloss"

	"github.com/tesinclair/hexdm/greeter"
)

var (
	accent  = lg.Color("#22E4FF")
	dim     = lg.Color("#7D7799")
	text    = lg.Color("#ECE8F7")
	bad     = lg.Color("#FF3B5C")
	good    = lg.Color("#3DFFA8")
	caution = lg.Color("#FFB454")

	labelStyle = lg.NewStyle().Foreground(dim).Width(10)
	valueStyle = lg.NewStyle().Foreground(text)
	focusStyle = lg.NewStyle().Foreground(accent).Bold(true)
	hintStyle  = lg.NewStyle().Foreground(dim)
	panelStyle = lg.NewStyle().
			Border(lg.RoundedBorder()).
			BorderForeground(dim).
			Padding(1, 4)
)

type tickMsg struct{}
type eventMsg struct{ ev greeter.Event }

type model struct {
	s      *greeter.State
	link   *greeter.Link
	width  int
	height int
	// The screen has been handed over to the session.
	released bool
}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Init() tea.Cmd { return tick() }

func toKey(msg tea.KeyMsg) (greeter.Key, bool) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return greeter.Key{Type: greeter.KeyInterrupt}, true
	case tea.KeyEnter:
		return greeter.Key{Type: greeter.KeyEnter}, true
	case tea.KeyEsc:
		return greeter.Key{Type: greeter.KeyEsc}, true
	case tea.KeyBackspace:
		return greeter.Key{Type: greeter.KeyBackspace}, true
	case tea.KeyLeft:
		return greeter.Key{Type: greeter.KeyLeft}, true
	case tea.KeyRight:
		return greeter.Key{Type: greeter.KeyRight}, true
	case tea.KeyUp:
		return greeter.Key{Type: greeter.KeyUp}, true
	case tea.KeyDown:
		return greeter.Key{Type: greeter.KeyDown}, true
	case tea.KeyRunes, tea.KeySpace:
		// Only typed (or pasted) characters are text. Every other key has
		// a name such as "tab", which must not be typed into the field.
		if !msg.Alt {
			return greeter.Key{Type: greeter.KeyText, Text: string(msg.Runes)}, true
		}
	}
	return greeter.Key{}, false
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	now := time.Now()

	switch msg := msg.(type) {
	case tickMsg:
		m.s.Tick(now)
		if !m.s.LoggedIn {
			// Once the session owns the screen, stop redrawing under it.
			cmd = tick()
		}
	case eventMsg:
		m.s.Handle(msg.ev, now)
	case tea.KeyMsg:
		if key, ok := toKey(msg); ok {
			m.s.Key(key, now)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	if m.s.LoggedIn && !m.released {
		// A terminal has nothing to give back, so the session can start
		// straight away.
		m.released = true
		m.link.Release()
	}
	if m.s.Done {
		return m, tea.Quit
	}
	return m, cmd
}

func (m model) View() string {
	s := m.s

	row := func(zone greeter.Zone, label, value string) string {
		style := valueStyle
		marker := "  "
		if s.Focus == zone && !s.LoggedIn {
			style = focusStyle
			marker = focusStyle.Render("> ")
		}
		return marker + labelStyle.Render(label) + style.Render(value)
	}
	arrows := func(value string, many bool) string {
		if many {
			return "‹ " + value + " ›"
		}
		return value
	}

	var login string
	switch {
	case s.Fprint && s.FprintFail:
		login = lg.NewStyle().Foreground(bad).Render("no match")
	case s.Fprint && s.Scanning:
		login = "reading fingerprint..."
	case s.Fprint:
		used := s.FprintMaxTries - s.FprintTries
		login = "touch the fingerprint reader  " +
			lg.NewStyle().Foreground(bad).Render(strings.Repeat("x", max(used, 0))) +
			strings.Repeat("o", max(s.FprintTries, 0))
	default:
		login = strings.Repeat("*", s.PasswordLen())
		if s.Insert {
			login += "_"
		}
	}

	shutdown, reboot := "shut down", "reboot"
	if s.PowerIdx == 0 {
		shutdown = "[" + shutdown + "]"
	} else {
		reboot = "[" + reboot + "]"
	}

	status, isBad := s.Status()
	statusStyle := lg.NewStyle().Foreground(text)
	switch {
	case s.Welcome():
		statusStyle = statusStyle.Foreground(good)
	case s.ErrMsg != "":
		statusStyle = statusStyle.Foreground(bad)
	case isBad:
		statusStyle = statusStyle.Foreground(caution)
	}

	mode := "NORMAL"
	if s.Insert {
		mode = "INSERT"
	}

	panel := panelStyle.Render(lg.JoinVertical(lg.Left,
		focusStyle.Render("HEX DM")+hintStyle.Render("   "+mode),
		"",
		row(greeter.ZoneUser, "user", arrows(s.User(), len(s.Users) > 1)),
		row(greeter.ZoneAuth, "login", login),
		row(greeter.ZoneSession, "session", arrows(s.Session(), len(s.Sessions) > 1)),
		row(greeter.ZonePower, "power", shutdown+"  "+reboot),
		"",
		statusStyle.Render(status),
	))

	board := lg.JoinVertical(lg.Center, panel, hintStyle.Render(s.Hint()))
	return lg.Place(m.width, m.height, lg.Center, lg.Center, board)
}

// Run shows the terminal greeter until main shuts it down, the session ends
// or (when debugging) the user quits.
func Run(s *greeter.State, link *greeter.Link, events <-chan greeter.Event) error {
	log.Println("Starting up TUI.")
	p := tea.NewProgram(model{s: s, link: link}, tea.WithAltScreen())

	// Send blocks until the program has started and is a no-op once it has
	// finished.
	go func() {
		for ev := range events {
			p.Send(eventMsg{ev})
		}
	}()

	if _, err := p.Run(); err != nil {
		return err
	}
	return s.ExitErr
}
