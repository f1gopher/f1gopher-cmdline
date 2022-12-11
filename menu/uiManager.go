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
	"f1gopher/f1gopher-cmdline/sessionUI"
	"f1gopher/f1gopher-cmdline/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/f1gopher/f1gopherlib"
	"github.com/f1gopher/f1gopherlib/Messages"
	"time"
)

type UIManager struct {
	ready         bool
	err           error
	currentWidth  int
	currentHeight int
	menu          *mainMenu
	currentUI     ui.Page
	sessionUI     sessionUI.SessionUI
	replayMenu    *replayMenu
	cache         string
	liveDelay     time.Duration
	servers       []string
	display       string
}

func NewUI(cache string, servers []string, liveDelay time.Duration, displayLive bool, version string) *UIManager {
	display := &UIManager{
		err:        nil,
		menu:       newMainMenu(servers, version),
		currentUI:  ui.MainMenu,
		replayMenu: newReplayMenu(),
		cache:      cache,
		liveDelay:  liveDelay,
		servers:    servers,
	}

	if displayLive {
		liveConnection := newLiveConnection(display.cache)

		// No live event so do nothing
		if liveConnection == nil {
			display.menu.message = "There is no live session currently happening."
			display.currentUI = ui.MainMenu
		} else {
			display.currentUI = ui.Live
			display.sessionUI = display.createSessionUI(newLiveConnection(display.cache), true)
		}
	}

	return display
}

func (m UIManager) Init() tea.Cmd {
	m.menu.Enter()
	return tick()
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Duration(time.Millisecond*500), func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m UIManager) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msgType := msg.(type) {
	case tickMsg:
		return m, tick()

	case tea.KeyMsg:
		switch msgType.Type {

		case tea.KeyCtrlC, tea.KeyCtrlBackslash:
			return m, tea.Quit

		default:
			switch m.currentUI {
			case ui.MainMenu:
				m.currentUI, cmds = m.menu.Update(msgType)

				if m.currentUI == ui.Quit {
					return m, tea.Quit
				}

				if m.currentUI == ui.Live {
					liveConnection := newLiveConnection(m.cache)

					// No live event so do nothing
					if liveConnection == nil {
						m.menu.message = "There is no live session currently happening."
						m.currentUI = ui.MainMenu
					} else {
						m.sessionUI = m.createSessionUI(newLiveConnection(m.cache), true)
					}
				}

			case ui.Live, ui.Replay:
				m.currentUI, cmds = m.sessionUI.Update(msgType)
				if m.currentUI != ui.Live && m.currentUI != ui.Replay {
					m.sessionUI.Leave()
					m.sessionUI = nil
					m.menu.Enter()
				}

			case ui.ReplayMenu:
				m.currentUI, cmds = m.replayMenu.Update(msgType)
				if m.currentUI == ui.Replay {
					m.sessionUI = m.createSessionUI(newReplayConnection(m.cache, m.replayMenu.choice.event), false)
				}
			}
		}

	case tea.WindowSizeMsg:

		m.currentWidth = msgType.Width
		m.currentHeight = msgType.Height
		m.ready = true

		m.menu.Resize(msgType)
		m.replayMenu.Resize(msgType)
		if m.sessionUI != nil {
			m.sessionUI.Resize(msgType)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m UIManager) View() string {
	switch m.currentUI {
	case ui.MainMenu:
		return m.menu.View()

	case ui.Live, ui.Replay:
		return m.sessionUI.View()

	case ui.ReplayMenu:
		return m.replayMenu.View()

	case ui.Quit:

	default:
		panic("Unhandled menu option")
	}

	return ""
}

func (m UIManager) createSessionUI(data f1gopherlib.F1GopherLib, isLive bool) sessionUI.SessionUI {

	var result sessionUI.SessionUI

	switch data.Session() {
	case Messages.Practice1Session, Messages.Practice2Session, Messages.Practice3Session, Messages.QualifyingSession:
		result = sessionUI.NewPracticeQualifyingUI(m.servers, m.liveDelay)

	case Messages.SprintSession, Messages.RaceSession:
		result = sessionUI.NewRaceUI(m.servers, m.liveDelay)

	default:
		panic("Unhandled session type: " + data.Session().String())
	}

	result.Enter(data, m.currentUI, isLive)
	return result
}
