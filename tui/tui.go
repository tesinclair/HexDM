package tui

import (
	"math/rand"
	"strings"
	"errors"
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	borderColor = lipgloss.Color("#431C55")
	textColor = lipgloss.Color("#E8D6CB")

	title = titles[rand.Intn(len(titles))]

	columnStyle = lipgloss.NewStyle().
							Padding(1, 2).
							Border(lipgloss.RoundedBorder()).
							BorderForeground(borderColor)
	
	titleStyle = lipgloss.NewStyle().
							Foreground(textColor).
							Bold(false).
							PaddingBottom(1)

)

var (
	UserQuit = errors.New("User Quit Session")
)

type Model struct{
	width int
	height int

	password chan string
	auth chan error 
	passwordText string
	errMsg string // for the user

	exitMsg error // for the main loop
}

type AuthMsg string

func New() *Model{
	m := &Model{}
	return m
}

func (m Model) Init() tea.Cmd{ return nil}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd){
	switch msg := msg.(type){
	case AuthMsg:
		if msg == "Login Successful"{
			m.errMsg = ""
			m.exitMsg = nil
			return m, tea.Quit
		}else{
			m.passwordText = ""
			m.errMsg = fmt.Sprintf("Login Failed: %v", string(msg))
		}
	case tea.KeyMsg:
		switch msg.String(){
		case "ctrl+c", "q":
			m.exitMsg = UserQuit
			return m, tea.Quit
		case "enter":
			if m.passwordText == ""{
				return m, nil
			}

			pass := m.passwordText
			return m, func() tea.Msg{
				m.password<- pass
				err := <-m.auth

				if err == nil{
					return AuthMsg("Login Successful")
				}else{
					return AuthMsg(fmt.Sprintf("%v", err))
				}
			}
		case "backspace":
			if len(m.passwordText) > 0{
				m.passwordText = m.passwordText[0:len(m.passwordText) -1]
			}
		default:
			m.passwordText = m.passwordText + msg.String()

		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m Model) View() string{
	titleRendered := titleStyle.Render(title)

	height := m.height - 4 - lipgloss.Height(titleRendered)
	if height < 0{ height = 0}
	width := lipgloss.Width(titleRendered)
	if width < 100{
		width = 100 // set a safe minimum
	}

	var style lipgloss.Style = columnStyle.Copy()
	style = style.Width(width).Height(height)


	password := titleStyle.Render(strings.Repeat("*", len(m.passwordText)))

	passwordBoxStyle := style.Copy().Height(1).Width(50).AlignVertical(lipgloss.Center)
	passwordBox := passwordBoxStyle.Render(password)

	box := style.Render(lipgloss.JoinVertical(lipgloss.Left, passwordBox, m.errMsg))

	board := lipgloss.JoinVertical(lipgloss.Center, titleStyle.Render(title), box)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, board)
}

func Render(kill chan bool, auth chan error, pass chan string) error{
	log.Println("Starting up TUI.")
	m := New()
	m.password = pass
	m.auth = auth

	p := tea.NewProgram(m, tea.WithAltScreen())

	done := make(chan error, 1)
	go func(){
		mod, err := p.Run()
		if err != nil{
			done<- err
			return
		}

		if m, ok := mod.(Model); ok{
			done<- m.exitMsg
		}else{
			done<- nil
		}
	}()

	select{
	case <-kill:
		p.Quit()
		<-done
		return nil
	case err := <-done:
		return err
	}
}



