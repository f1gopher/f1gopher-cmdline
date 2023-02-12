// F1Gopher-CmdLine - Copyright (C) 2022 f1gopher
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package menu

import (
	"f1gopher/f1gopher-cmdline/ui"
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
	version     string
}

func newMainMenu(servers []string, version string) *mainMenu {

	menu := []string{
		"Live",
		"Replay",
		"Quit"}

	return &mainMenu{
		cursor:  0,
		choices: menu,
		servers: strings.Join(servers, ","),
		version: version,
	}
}

func (m *mainMenu) Resize(msg tea.WindowSizeMsg) {
	m.currentWidth = msg.Width
	m.currentHeight = msg.Height
}

func (m *mainMenu) Enter() {
	nextSession, _, _, _ := f1gopherlib.HappeningSessions()
	m.nextSession = fmt.Sprintf("%s %s at %s",
		nextSession.Name,
		strings.Replace(nextSession.Type.String(), "_", " ", -1),
		nextSession.EventTime.In(time.Local).Format("15:04 02 Jan 2006 MST"))
}

func (m *mainMenu) Update(msg tea.Msg) (newUI ui.Page, cmds []tea.Cmd) {
	newUI = ui.MainMenu

	switch msgType := msg.(type) {
	case tea.KeyMsg:
		switch msgType.String() {
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
				newUI = ui.Live

			case "Replay":
				newUI = ui.ReplayMenu

			case "Quit":
				newUI = ui.Quit
			}
		}
	}

	return newUI, nil
}

func (m *mainMenu) View() string {

	s := lipgloss.NewStyle().
		Underline(true).
		Foreground(lipgloss.Color("#AF0202")).
		Render("F1Gopher-Cmdline") + "\n\n"

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

	serverInfo := lipgloss.NewStyle().Width(m.currentWidth).Align(lipgloss.Right).Render(
		fmt.Sprintf("Server(s): %s", m.servers))
	version := lipgloss.NewStyle().Width(m.currentWidth).Align(lipgloss.Right).Render("v" + m.version)

	return fmt.Sprintf("%s\n%s\n%s", menu, serverInfo, version)
}
