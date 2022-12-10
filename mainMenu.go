package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/f1gopher/f1gopherlib"
	"strings"
	"time"
)

var menuSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
var dialogBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#874BFD")).
	Padding(1, 10).
	BorderTop(true).
	BorderLeft(true).
	BorderRight(true).
	BorderBottom(true)
var subtle = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}

type mainMenu struct {
	cursor  int
	choices []string

	currentWidth  int
	currentHeight int

	message     string
	servers     string
	nextSession string
}

func NewMainMenu(hasDebugFile bool, servers []string) *mainMenu {

	menu := []string{
		"Live",
		"Replay",
		"Quit"}

	if hasDebugFile {
		menu = []string{
			"Live",
			"Replay",
			"Debug Replay",
			"Quit"}
	}

	return &mainMenu{
		cursor:  0,
		choices: menu,
		servers: strings.Join(servers, ","),
	}
}

func (m *mainMenu) Resize(msg tea.WindowSizeMsg) {
	m.currentWidth = msg.Width
	m.currentHeight = msg.Height
}

func (m *mainMenu) Enter() {
	nextSession, _ := f1gopherlib.NextSession()
	m.nextSession = fmt.Sprintf("%s %s at %s",
		nextSession.Name,
		strings.Replace(nextSession.Type.String(), "_", " ", -1),
		nextSession.EventTime.In(time.Local).Format("15:04 02 Jan 2006 MST"))
}

func (m *mainMenu) Update(msg tea.Msg) (newUI uiPage, cmds []tea.Cmd) {
	newUI = MainMenu

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			switch m.choices[m.cursor] {
			case "Live":
				newUI = Live

			case "Replay":
				newUI = ReplayMenu

			case "Debug Replay":
				newUI = DebugReplay

			case "Quit":
				newUI = Quit

			default:
				panic("")
			}
		}
	}

	return newUI, nil
}

func (m *mainMenu) View() string {

	s := lipgloss.NewStyle().
		Underline(true).
		Foreground(lipgloss.Color("#AF0202")).
		Render("F1Gopher") + "\n\n"

	s += lipgloss.NewStyle().Render("Next Session: ")

	s += lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FFFF")).
		Render(m.nextSession) + "\n\n"

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			s += menuSelected.Render(fmt.Sprintf("> %s", choice)) + "\n"
		} else {
			s += fmt.Sprintf("%s %s", cursor, choice) + "\n"
		}
	}

	var menu string
	if len(m.message) > 0 {
		menu = lipgloss.Place(m.currentWidth, m.currentHeight-11,
			lipgloss.Center, lipgloss.Center,
			dialogBoxStyle.Render(s),
			lipgloss.WithWhitespaceForeground(subtle),
		)

		menu += "\n\n\n\n" + lipgloss.NewStyle().Width(m.currentWidth).Align(lipgloss.Center).Render(m.message)
	} else {
		menu = lipgloss.Place(m.currentWidth, m.currentHeight-2,
			lipgloss.Center, lipgloss.Center,
			dialogBoxStyle.Render(s),
			lipgloss.WithWhitespaceForeground(subtle),
		)
	}

	serverInfo := lipgloss.NewStyle().Width(m.currentWidth).Align(lipgloss.Right).Render(fmt.Sprintf("Server(s): %s", m.servers))
	version := lipgloss.NewStyle().Width(m.currentWidth).Align(lipgloss.Right).Render("v0.5")

	return fmt.Sprintf("%s\n%s\n%s", menu, serverInfo, version)
}
