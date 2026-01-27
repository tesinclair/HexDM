package tui

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	lg "github.com/charmbracelet/lipgloss"
)

var (
	borderColor         = lg.Color("#444444")
	borderFocussedColor = lg.Color("#431C55")

	textColor = lg.Color("#E8D6CB")

	title = titles[rand.Intn(len(titles))]

	columnStyle = lg.NewStyle().
			Padding(1, 2).
			Border(lg.RoundedBorder()).
			BorderForeground(borderColor)

	titleStyle = lg.NewStyle().
			Foreground(textColor).
			Bold(false).
			PaddingBottom(1)
)

var (
	UserQuit         = errors.New("User Quit Session")
	ErrSessionFailed = errors.New("Failed To Launch Your Session.")
)

var normalMode = true 

type Model struct {
	width  int
	height int

	users   []string
	userIdx int

	components []string
	componentIdx int

	user         chan string
	password     chan string
	auth         chan error
	passwordText string
	errMsg       string // for the user

	exitMsg error // for the main loop

	focussed string
}

type authMsg struct {
	msg string
	err error
}

func New() *Model {
	m := &Model{
		userIdx:  0,
		componentIdx: 0,
		components: []string{
			"users",
			"password",
		},
	}
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case authMsg:
		if msg.msg == "Login Successful" {
			m.errMsg = ""
			m.exitMsg = nil
			return m, tea.Quit
		} else if msg.msg == "Session Error" {
			m.errMsg = fmt.Sprintf("Session Error: %v", msg.err)
		} else {
			m.passwordText = ""
			m.errMsg = fmt.Sprintf("Login Failed: %v", msg.err)
		}
	case tea.KeyMsg:
		if normalMode {
			switch msg.String() {
			case "ctrl+c", "q":
				if os.Getenv("HEX_DEBUG") == "1"{
					m.exitMsg = UserQuit
					return m, tea.Quit
				}
			case "left", "h":
				m.userIdx = m.userIdx - 1
				if m.userIdx < 0 {
					m.userIdx = len(m.users) - 1
				}
			case "right", "l":
				m.userIdx = (m.userIdx + 1) % len(m.users)
			case "up", "k":
				m.componentIdx = m.componentIdx - 1
				if m.componentIdx < 0 {
					m.componentIdx = len(m.components) - 1
				}
			case "down", "j":
				m.componentIdx = (m.componentIdx + 1) % len(m.components)	
			case "i":
				normalMode = false
			}

		} else {
			switch msg.String() {
			case "ctrl+c":
				if os.Getenv("HEX_DEBUG") == "1"{
					m.exitMsg = UserQuit
					return m, tea.Quit
				}
			case "esc":
				normalMode = true
				m.focussed = "users"
			case "enter":
				if m.passwordText == "" {
					return m, nil
				}

				pass := m.passwordText
				user := m.users[m.userIdx]
				return m, func() tea.Msg {
					m.password <- pass
					m.user <- user
					err := <-m.auth

					if err == nil {
						return authMsg{
							msg: "Login Successful",
							err: nil,
						}
					} else if errors.Is(err, ErrSessionFailed) {
						return authMsg{
							msg: "Session Error",
							err: err,
						}
					} else {
						return authMsg{
							msg: "Generic Error",
							err: err,
						}
					}
				}
			case "backspace":
				if len(m.passwordText) > 0 {
					m.passwordText = m.passwordText[0 : len(m.passwordText)-1]
				}
			default:
				m.passwordText = m.passwordText + msg.String()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	m.focussed = m.components[m.componentIdx]

	return m, nil
}

func (m Model) View() string {
	titleRendered := titleStyle.Render(title)

	height := m.height - 4 - lg.Height(titleRendered)
	if height < 0 {
		height = 0
	}
	width := lg.Width(titleRendered)
	if width < 100 {
		width = 100 // set a safe minimum
	}

	var style lg.Style = columnStyle.Copy()
	style = style.Width(width).Height(height)

	// Password
	password := titleStyle.Render(strings.Repeat("*", len(m.passwordText)))
	tag := titleStyle.Foreground(borderFocussedColor).Bold(true).Render("> ")
	passwordBoxStyle := style.Copy().Height(1).Width(50).AlignVertical(lg.Center)
	if m.focussed == "password" {
		passwordBoxStyle = passwordBoxStyle.BorderForeground(borderFocussedColor)
	}
	var passwordBox string;
	if m.focussed == "password" && !normalMode{
		passwordBox = passwordBoxStyle.Render(lg.JoinHorizontal(lg.Center, tag, password))
	}else{
		passwordBox = passwordBoxStyle.Render(password)
	}

	// Users
	user := titleStyle.Render(strings.ToUpper(m.users[m.userIdx]))
	userBoxStyle := style.Copy().Height(1).Width(50).Align(lg.Center)
	if m.focussed == "users" {
		userBoxStyle = userBoxStyle.BorderForeground(borderFocussedColor)
	}
	userBox := userBoxStyle.Bold(true).Render(user)

	box := style.Align(lg.Center).Render(lg.JoinVertical(lg.Center, userBox, passwordBox, m.errMsg))

	board := lg.JoinVertical(lg.Center, titleStyle.Render(title), box)

	return lg.Place(m.width, m.height, lg.Center, lg.Center, board)
}

func Render(users []string, kill chan bool, auth chan error, user chan string, pass chan string) error {
	log.Println("Starting up TUI.")
	m := New()
	m.password = pass
	m.auth = auth
	m.users = users
	m.user = user

	p := tea.NewProgram(m, tea.WithAltScreen())

	done := make(chan error, 1)
	go func() {
		mod, err := p.Run()
		if err != nil {
			done <- err
			return
		}

		if m, ok := mod.(Model); ok {
			done <- m.exitMsg
		} else {
			done <- nil
		}
	}()

	select {
	case <-kill:
		p.Quit()
		<-done
		return nil
	case err := <-done:
		return err
	}
}
