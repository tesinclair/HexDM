package tui

import (
	"math/rand"

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

type model struct{
	width int
	height int
}

func New() *model{
	m := &model{}
	return m
}

func (m model) Init() tea.Cmd{ return nil}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd){
	switch msg := msg.(type){
	case tea.KeyMsg:
		switch msg.String(){
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m model) View() string{
	titleRendered := titleStyle.Render(title)

	height := m.height - 4 - lipgloss.Height(titleRendered)
	if height < 0{ height = 0}
	width := lipgloss.Width(titleRendered)
	if width < 100{
		width = 100 // set a safe minimum
	}

	var style lipgloss.Style = columnStyle.Copy()
	style = style.Width(width).Height(height)

	box := style.Render("")

	board := lipgloss.JoinVertical(lipgloss.Center, titleStyle.Render(title), box)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, board)
}

func Render(kill chan bool) error{
	m := New()

	p := tea.NewProgram(m, tea.WithAltScreen())

	done := make(chan error)
	go func(){
		_, err := p.Run()
		done<- err
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



