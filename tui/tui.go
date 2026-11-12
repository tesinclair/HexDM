package tui

import (
	"bytes"
	"errors"
	"fmt"
	"time"
	"log"
	"math/rand"
	"os"
	"strings"
	"unicode/utf8"

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
	ErrFingerprintNoMatch = errors.New("Couldn't Match Fingerprint")
)

const (
	PowerShutdown = "shutdown"
	PowerReboot = "reboot"
)

var normalMode = true

// Longest password the field accepts, in bytes.
const maxPasswordLen = 512

// wipe zeroes the whole password buffer and returns it empty.
func wipe(buf []byte) []byte {
	buf = buf[:cap(buf)]
	clear(buf)
	return buf[:0]
}

type Model struct {
	width  int
	height int

	users   []string
	userIdx int

	sessions []string
	sessionIdx int

	components   []string
	componentIdx int

	options []string
	optionsIdx int

	optionFocus string

	user         chan string
	session      chan string
	password     chan []byte
	auth         chan error
	// The password is kept in one fixed buffer that is never reallocated,
	// so it can be wiped. (Model is copied by value, but every copy shares
	// this buffer.)
	passwordText []byte
	errMsg       string // for the user

	fprintFail bool
	fprintdAvail  chan bool
	fprintdTriesChan chan int
	fprintdTries  int // -1 means unavailable
	fprintdMaxTries int 
	fprintdActive bool
	scannerActive chan bool
	isScannerActive bool
	scannerFrame int
	powerReq chan string // chan to send power requests to main
	out chan func() // sends to main, delivered one at a time in order
	dpIdxChan chan int

	exitMsg    error // for the main loop
	warningMsg string

	focussed string
	activeSession string
}

// toMain queues a send to the main loop. A single goroutine performs the
// queued sends one after another, so main sees a user change, a session
// choice and a password in the order they were made, and Update never blocks
// on a busy main loop. It reports false if the queue is full and the send was
// dropped.
func (m Model) toMain(send func()) bool {
	select {
	case m.out <- send:
		return true
	default:
		log.Println("Main loop is not keeping up. Dropping input.")
		return false
	}
}

type authMsg struct {
	msg string
	err error
}

type fprintMsg bool
type warningMsg string
type fprintdTriesMsg int
type fprintFailMsg bool
type scannerActiveMsg bool
type tickMsg bool
type dpIdxMsg int

func New() *Model {
	m := &Model{
		userIdx:      0,
		sessionIdx: 0,
		componentIdx: 0,
		components: []string{
			"users",
			"login",
			"options",
		},
		optionsIdx: 0,
		passwordText: make([]byte, 0, maxPasswordLen),
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
		return tickMsg(true) // start ticker
	}
}

// newAuthMsg turns a value from main's auth channel into a message.
func newAuthMsg(err error) authMsg {
	switch {
	case err == nil:
		return authMsg{msg: "Login Successful"}
	case errors.Is(err, ErrSessionFailed):
		return authMsg{msg: "Session Error", err: err}
	default:
		return authMsg{msg: "Generic Error", err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case dpIdxMsg:
		if idx := int(msg); idx >= 0 && idx < len(m.sessions) {
			m.sessionIdx = idx
			m.activeSession = m.sessions[m.sessionIdx]
		}
	case tickMsg:
		m.scannerFrame = (m.scannerFrame + 1) % 100 // max int protection
		cmds = append(cmds, func() tea.Msg{
			time.Sleep(100 * time.Millisecond)
			return tickMsg(true)
		})
	case scannerActiveMsg:
		if !msg{
			m.scannerFrame = 0
		}
		m.isScannerActive = bool(msg)
	case fprintFailMsg:
		m.fprintFail = bool(msg)
	case fprintdTriesMsg:
		m.fprintdTries = int(msg)
		if m.fprintdTries > m.fprintdMaxTries {
			m.fprintdMaxTries = m.fprintdTries
		}
		if m.fprintdTries < 1 {
			// Out of tries: fall back to the password box.
			m.fprintdActive = false
		}
	case fprintMsg:
		if msg {
			m.fprintdActive = true
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
			cmds = append(cmds, func() tea.Msg{
				time.Sleep(2 * time.Second)
				return warningMsg("")
			})
		}else{
			m.warningMsg = ""
		}
	case authMsg:
		if msg.msg ==""{
			m.errMsg = ""
			m.exitMsg = nil
		} else if msg.msg == "Login Successful" {
			m.errMsg = ""
			m.exitMsg = nil
			return m, tea.Quit
		} else if msg.msg == "Session Error" {
			// Stays up until main shuts us down.
			m.errMsg = fmt.Sprintf("Session Error: %s", msg.err)
		} else {
			if errors.Is(msg.err, ErrFingerprintNoMatch) {
				// Flash the red fingerprint.
				m.fprintFail = true
				cmds = append(cmds, func() tea.Msg{
					time.Sleep(1 * time.Second)
					return fprintFailMsg(false)
				})
			}
			m.errMsg = fmt.Sprintf("Login Failed: %s", msg.err)
			cmds = append(cmds, func() tea.Msg{
				time.Sleep(2 * time.Second)
				return authMsg{}
			})
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
					m.passwordText = wipe(m.passwordText)
					m.toMain(func() { m.user <- user })
				case "login":
					if m.fprintdTries > 0 {
						m.fprintdActive = !m.fprintdActive
					} else {
						cmds = append(cmds, func() tea.Msg {
							return warningMsg("Fingerprint not available for current user")
						})
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
							m.sessionIdx = m.sessionIdx - 1
							if m.sessionIdx < 0{
								m.sessionIdx = len(m.sessions) - 1
							}
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
					m.passwordText = wipe(m.passwordText)
					m.toMain(func() { m.user <- user })
				case "login":
					if m.fprintdTries > 0 {
						m.fprintdActive = !m.fprintdActive
					} else {
						cmds = append(cmds, func() tea.Msg {
							return warningMsg("Fingerprint not available for current user")
						})
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
							if m.optionFocus == "session"{
								m.sessionIdx = (m.sessionIdx + 1) % len(m.sessions)
							}
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
					switch(m.options[m.optionsIdx]){
					case "power":
						if m.optionFocus == "off"{
							m.toMain(func() { m.powerReq <- PowerShutdown })
						}else if m.optionFocus == "reboot"{
							m.toMain(func() { m.powerReq <- PowerReboot })
						}
					case "session":
						if m.optionFocus == "session"{
							m.activeSession = m.sessions[m.sessionIdx]
							chosen := m.activeSession
							m.toMain(func() { m.session <- chosen })
						}
					}
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

					if len(m.passwordText) == 0 {
						return m, nil
					}

					// Hand main its own copy and wipe ours straight away.
					// Main wipes its copy once PAM has seen it.
					pass := bytes.Clone(m.passwordText)
					m.passwordText = wipe(m.passwordText)
					if !m.toMain(func() { m.password <- pass }) {
						clear(pass)
					}
				case "options":
					if m.options[m.optionsIdx] == "power"{
						if m.optionFocus == "off"{
							m.toMain(func() { m.powerReq <- PowerShutdown })
						}else if m.optionFocus == "reboot"{
							m.toMain(func() { m.powerReq <- PowerReboot })
						}
					}
				}
				
			case "backspace":
				// Remove one whole character, not one byte.
				_, size := utf8.DecodeLastRune(m.passwordText)
				keep := len(m.passwordText) - size
				clear(m.passwordText[keep:])
				m.passwordText = m.passwordText[:keep]
			default:
				// Only typed (or pasted) characters belong in the password.
				// Every other key has a name such as "left" or "tab", which
				// must not be typed into the field.
				if (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) && !msg.Alt {
					typed := []byte(string(msg.Runes))
					if len(m.passwordText)+len(typed) <= cap(m.passwordText) {
						m.passwordText = append(m.passwordText, typed...)
					}
					clear(typed)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	m.focussed = m.components[m.componentIdx]
	return m, tea.Batch(cmds...)	
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
		password := titleStyle.Render(strings.Repeat("*", utf8.RuneCount(m.passwordText)))
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
		fprintIcon := fprintIcon
		fprintStyle := titleStyle.Copy()
		if m.isScannerActive{
			fprintStyle = fprintStyle.Foreground(lg.Color("12"))
			fprintIcon = fprintScanningFrames[m.scannerFrame % len(fprintScanningFrames)]
		}
		if m.fprintFail{
			fprintStyle = fprintStyle.Foreground(lg.Color("9"))
			fprintIcon = fprintFailIcon
		}
		fprint := fprintStyle.Render(fprintIcon)
		fprintBoxStyle := style.Copy().Height(lg.Height(fprint) - 2).Width(lg.Width(fprint) + 4).Border(lg.HiddenBorder())
		if m.focussed == "login" {
			fprintStyle = fprintStyle.Foreground(borderFocussedColor)
		}
		loginBox = fprintBoxStyle.Render(fprint)
	}

	// Authentication fprint attempts
	var tryBox string
	if m.fprintdTries > 0{
		failedAttempts := m.fprintdMaxTries - m.fprintdTries	
		tryboxStyle := style.Copy().Width(1).Height(1).AlignVertical(lg.Center).Border(lg.ThickBorder(), true, true)
		tryboxFailedStyle := tryboxStyle.Copy().BorderForeground(lg.Color("9")).Foreground(lg.Color("9"))

		var tryBoxes []string
		for _ = range failedAttempts{
			tryBoxes = append(tryBoxes, tryboxFailedStyle.Render("X"))
		}
		for _ = range m.fprintdTries{
			tryBoxes = append(tryBoxes, tryboxStyle.Render(""))
		}

		tryBoxJoinedStyle := style.Copy().Width(25).Height(1).Border(lg.HiddenBorder())

		tryBox = tryBoxJoinedStyle.Render(lg.JoinHorizontal(lg.Center, tryBoxes...))
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
		sessionLabelStyle := titleStyle.Copy()
		sessionText := m.sessions[m.sessionIdx]
		if sessionText == m.activeSession{
			sessionLabelStyle = sessionLabelStyle.Foreground(borderFocussedColor)
		}
		sessionLabel := sessionLabelStyle.Render(sessionText)
		optionSubBox = sessionBox.Render(lg.JoinHorizontal(lg.Center, sessionLabel))
	}
	optionsBoxStyle := style.Copy().Height(1).Width(50).AlignVertical(lg.Center).Align(lg.Center)
	if m.focussed == "options"{
		optionsBoxStyle = optionsBoxStyle.BorderForeground(borderFocussedColor)
	}
	optionsLabel := strings.ToUpper(m.options[m.optionsIdx])
	optionsBox := optionsBoxStyle.Render(optionsLabel)

	topContentItems := lg.JoinVertical(lg.Center, userBox, loginBox, m.errMsg, m.warningMsg, tryBox)
	topContentHeight := lg.Height(topContentItems)
	topContent := lg.PlaceVertical(max(height - topContentHeight, 0), lg.Top, topContentItems)

	box := style.Align(lg.Center).Render(lg.JoinVertical(lg.Center, topContent, optionsBox, optionSubBox))

	board := lg.JoinVertical(lg.Center, titleStyle.Render(title), box)

	return lg.Place(m.width, m.height, lg.Center, lg.Center, board)
}

func Render(sessions, users []string, fprintd, kill, scannerActive chan bool, auth chan error, user chan string, pass chan []byte, session, power chan string, fprintdTriesChan, dpIdxChan chan int) error {
	log.Println("Starting up TUI.")
	if len(sessions) == 0 || len(users) == 0 {
		return errors.New("TUI needs at least one user and one session")
	}
	m := New()
	m.password = pass
	m.auth = auth
	m.users = users
	m.user = user
	m.fprintdAvail = fprintd
	m.fprintdTriesChan = fprintdTriesChan
	m.fprintdTries = <-fprintdTriesChan
	m.fprintdMaxTries = m.fprintdTries
	m.powerReq = power
	m.sessions = sessions
	m.session = session
	m.scannerActive = scannerActive
	m.scannerFrame = 0
	m.activeSession = m.sessions[0]
	m.dpIdxChan = dpIdxChan

	m.fprintdActive = <-m.fprintdAvail // wait for the first

	m.out = make(chan func(), 256)
	go func() {
		for send := range m.out {
			send()
		}
	}()

	p := tea.NewProgram(m, tea.WithAltScreen())

	// One permanent reader per channel coming from main. Each value becomes a
	// message for Update, so a send from main is always picked up, whatever
	// state the UI is in. Send blocks until the program has started and is a
	// no-op once it has finished.
	go func() {
		for err := range auth {
			p.Send(newAuthMsg(err))
		}
	}()
	go func() {
		for avail := range fprintd {
			p.Send(fprintMsg(avail))
		}
	}()
	go func() {
		for tries := range fprintdTriesChan {
			p.Send(fprintdTriesMsg(tries))
		}
	}()
	go func() {
		for active := range scannerActive {
			p.Send(scannerActiveMsg(active))
		}
	}()
	go func() {
		for idx := range dpIdxChan {
			p.Send(dpIdxMsg(idx))
		}
	}()

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
