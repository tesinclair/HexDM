package tui

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	lg "github.com/charmbracelet/lipgloss"
	figure "github.com/common-nighthawk/go-figure"
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

const (
	PowerShutdown = "shutdown"
	PowerReboot = "reboot"
)

var normalMode = true

type Model struct {
	width  int
	height int

	users   []string
	userIdx int

	components   []string
	componentIdx int

	options []string
	optionsIdx int

	optionFocus string

	user         chan string
	password     chan string
	auth         chan error
	passwordText string
	errMsg       string // for the user

	fprintdAvail  chan bool
	fprintdTries  int // -1 means unavailable
	fprintdActive bool
	powerReq chan string // chan to send power requests to main

	exitMsg    error // for the main loop
	warningMsg string

	focussed string
}

type authMsg struct {
	msg string
	err error
}

type fprintMsg bool
type warningMsg string

func New() *Model {
	m := &Model{
		userIdx:      0,
		componentIdx: 0,
		components: []string{
			"users",
			"login",
			"options",
		},
		optionsIdx: 0,
		options: []string{
			"options",
			"power",
			"session",
		},
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		select {
		case err := <-m.auth:
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
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {
	case fprintMsg:
		if msg {
			m.fprintdActive = true
			m.fprintdTries = 3
			m.warningMsg = ""
			m.errMsg = ""
		} else {
			m.fprintdActive = false
			m.fprintdTries = -1
			m.warningMsg = ""
			m.errMsg = ""
		}
	case warningMsg:
		if msg != "" {
			m.warningMsg = string(msg)
		}
	case authMsg:
		if msg.msg == "Login Successful" {
			m.errMsg = ""
			m.exitMsg = nil
			return m, tea.Quit
		} else if msg.msg == "Session Error" {
			m.errMsg = fmt.Sprintf("Session Error: %s", msg.err)
		} else {
			m.passwordText = ""
			m.errMsg = fmt.Sprintf("Login Failed: %s", msg.err)
		}
		return m, func() tea.Msg {
			select {
			case err := <-m.auth:
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
		}
	case tea.KeyMsg:
		if normalMode {
			switch msg.String() {
			case "ctrl+c", "q":
				if os.Getenv("HEX_DEBUG") == "1" {
					m.exitMsg = UserQuit
					return m, tea.Quit
				}
			case "left", "h":
				switch m.focussed {
				case "users":
					m.userIdx = m.userIdx - 1
					if m.userIdx < 0 {
						m.userIdx = len(m.users) - 1
					}
					user := m.users[m.userIdx]
					return m, func() tea.Msg {
						m.user <- user
						fprintd := <-m.fprintdAvail
						return fprintMsg(fprintd)
					}
				case "login":
					if m.fprintdTries > 0 {
						m.fprintdActive = !m.fprintdActive
					} else {
						return m, func() tea.Msg {
							return warningMsg("Fingerprint not available for current user")
						}
					}
				case "options":
					if m.optionFocus != ""{
						switch (m.options[m.optionsIdx]){
						case "power":
							if m.optionFocus == "off"{
								m.optionFocus = "reboot"
							}else{
								m.optionFocus = "off"
							}
						case "session":
						}
					}else{
						m.optionsIdx = m.optionsIdx - 1
						if m.optionsIdx < 0{
							m.optionsIdx = len(m.options) - 1
						}
					}
				}
			case "right", "l":
				switch m.focussed {
				case "users":
					m.userIdx = (m.userIdx + 1) % len(m.users)
					user := m.users[m.userIdx]
					return m, func() tea.Msg {
						m.user <- user
						fprintd := <-m.fprintdAvail
						return fprintMsg(fprintd)
					}
				case "login":
					if m.fprintdTries > 0 {
						m.fprintdActive = !m.fprintdActive
					} else {
						return m, func() tea.Msg {
							return warningMsg("Fingerprint not available for current user")
						}
					}
				case "options":
					if m.optionFocus != ""{
						switch (m.options[m.optionsIdx]){
						case "power":
							if m.optionFocus == "off"{
								m.optionFocus = "reboot"
							}else{
								m.optionFocus = "off"
							}
						case "session":
						}
					}else{
						m.optionsIdx = (m.optionsIdx + 1) % len(m.options)
					}
					
				}
			case "up", "k":
				if m.focussed == "options" && m.optionFocus != ""{
					m.optionFocus = ""
				}else{
					m.componentIdx = m.componentIdx - 1
					if m.componentIdx < 0 {
						m.componentIdx = len(m.components) - 1
					}
				}
			case "down", "j":
				if m.focussed == "options" && m.optionFocus == ""{
					switch(m.options[m.optionsIdx]){
					case "power":
						m.optionFocus = "off"
					case "session":
						m.optionFocus = "session"
					default:
						m.componentIdx = (m.componentIdx + 1) % len(m.components)
						m.optionFocus = ""
					}
				 }else{
					m.componentIdx = (m.componentIdx + 1) % len(m.components)
					m.optionFocus = ""
				}
			case "i":
				if m.focussed == "login" && !m.fprintdActive {
					normalMode = false
				}

			case "enter":
				if m.focussed == "options"{
					if m.options[m.optionsIdx] == "power"{
						if m.optionFocus == "off"{
							m.powerReq <- "shutdown"
						}else if m.optionFocus == "reboot"{
							m.powerReq <- "reboot"
						}
					}
					return m, nil

				}
			}


		} else {
			switch msg.String() {
			case "ctrl+c":
				if os.Getenv("HEX_DEBUG") == "1" {
					m.exitMsg = UserQuit
					return m, tea.Quit
				}
			case "esc":
				normalMode = true
			case "enter":
				switch(m.focussed){
				case "login":
					if m.fprintdActive {
						return m, nil
					}

					if m.passwordText == "" {
						return m, nil
					}

					pass := m.passwordText
					return m, func() tea.Msg {
						m.password <- pass
						return nil
					}
				case "options":
					if m.options[m.optionsIdx] == "power"{
						if m.optionFocus == "off"{
							m.powerReq <- "shutdown"
						}else if m.optionFocus == "reboot"{
							m.powerReq <- "reboot"
						}
					}
					return m, nil
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

	// Authentication
	var loginBox string
	if !m.fprintdActive {
		password := titleStyle.Render(strings.Repeat("*", len(m.passwordText)))
		tag := titleStyle.Foreground(borderFocussedColor).Bold(true).Render("> ")
		passwordBoxStyle := style.Copy().Height(1).Width(50).AlignVertical(lg.Center)
		if m.focussed == "login" {
			passwordBoxStyle = passwordBoxStyle.BorderForeground(borderFocussedColor)
		}
		if m.focussed == "login" && !normalMode {
			loginBox = passwordBoxStyle.Render(lg.JoinHorizontal(lg.Center, tag, password))
		} else {
			loginBox = passwordBoxStyle.Render(password)
		}
	} else {
		fprint := titleStyle.Render(fprintIcon)
		fprintBoxStyle := style.Copy().Height(lg.Height(fprint) - 2).Width(lg.Width(fprint) + 4)
		if m.focussed == "login" {
			fprintBoxStyle = fprintBoxStyle.BorderForeground(borderFocussedColor)
		}
		loginBox = fprintBoxStyle.Render(fprint)
	}

	// Users
	user := titleStyle.Copy()
	if m.focussed == "users" {
		user = user.Foreground(borderFocussedColor)
	}
	username := strings.ToUpper(m.users[m.userIdx])
	usernameAscii := figure.NewFigure(username, "big", true).String()

	if usernameAscii != "" {
		username = usernameAscii
	}

	userBox := user.Render(username)

	// Options
	var optionSubBox string
	switch(m.options[m.optionsIdx]){
	case "power":
		powerOffBoxStyle := style.Copy().Height(1).Width(25).AlignVertical(lg.Center).Align(lg.Center)
		powerRebootBoxStyle := style.Copy().Height(1).Width(25).AlignVertical(lg.Center).Align(lg.Center)
		if m.optionFocus == "off"{
			powerOffBoxStyle = powerOffBoxStyle.BorderForeground(borderFocussedColor)
		}
		if m.optionFocus == "reboot"{
			powerRebootBoxStyle = powerRebootBoxStyle.BorderForeground(borderFocussedColor)
		}
		optionSubBox = lg.JoinHorizontal(lg.Center, powerOffBoxStyle.Render("SHUTDOWN"), powerRebootBoxStyle.Render("REBOOT"))
	case "session":
		sessionBox := style.Copy().Height(1).Width(50).AlignVertical(lg.Center).Align(lg.Center)
		if m.optionFocus == "session"{
			sessionBox = sessionBox.BorderForeground(borderFocussedColor)
		}
		optionSubBox = sessionBox.Render("NOT IMPLEMENTED")
	}
	optionsBoxStyle := style.Copy().Height(1).Width(50).AlignVertical(lg.Center).Align(lg.Center)
	if m.focussed == "options"{
		optionsBoxStyle = optionsBoxStyle.BorderForeground(borderFocussedColor)
	}
	optionsLabel := strings.ToUpper(m.options[m.optionsIdx])
	optionsBox := optionsBoxStyle.Render(optionsLabel)

	topContent := lg.PlaceVertical(height, lg.Top, lg.JoinVertical(lg.Center, userBox, loginBox, m.errMsg, m.warningMsg))

	box := style.Align(lg.Center).Render(lg.JoinVertical(lg.Center, topContent, optionsBox, optionSubBox))

	board := lg.JoinVertical(lg.Center, titleStyle.Render(title), box)

	return lg.Place(m.width, m.height, lg.Center, lg.Center, board)
}

func Render(users []string, fprintd, kill chan bool, auth chan error, user, pass, power chan string) error {
	log.Println("Starting up TUI.")
	m := New()
	m.password = pass
	m.auth = auth
	m.users = users
	m.user = user
	m.fprintdAvail = fprintd
	m.fprintdTries = 3
	m.powerReq = power

	m.fprintdActive = <-m.fprintdAvail // wait for the first

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
