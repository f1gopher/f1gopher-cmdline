package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"time"
)

type uiPage int

const (
	MainMenu uiPage = iota
	ReplayMenu
	Live
	Replay
	DebugReplay
	Quit
)

type ui struct {
	ready         bool
	err           error
	currentWidth  int
	currentHeight int
	menu          *mainMenu
	currentUI     uiPage
	replayUI      *replayUI
	replayMenu    *replayMenu
	cache         string
	debugFile     string

	display string
}

func NewUI(cache string, debugFile string, servers []string, liveDelay time.Duration, displayLive bool) *ui {
	display := &ui{
		err:        nil,
		menu:       NewMainMenu(len(debugFile) > 0, servers),
		replayUI:   NewReplayUI(servers, liveDelay),
		currentUI:  MainMenu,
		replayMenu: NewReplayMenu(),
		cache:      cache,
		debugFile:  debugFile,
	}

	if displayLive {
		liveConnection := NewLiveConnection(display.cache)

		// No live event so do nothing
		if liveConnection == nil {
			// TODO - message for user
			display.menu.message = "There is no live session currently happening."
			display.currentUI = MainMenu
		} else {
			display.currentUI = Live
			display.replayUI.Enter(liveConnection, display.currentUI, true)
		}
	}

	return display
}

func (m ui) Init() tea.Cmd {
	m.menu.Enter()
	return tick()
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Duration(time.Millisecond*500), func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m ui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		return m, tick()

	case tea.KeyMsg:
		switch msg.Type {

		case tea.KeyCtrlC, tea.KeyCtrlBackslash:
			return m, tea.Quit

		default:
			switch m.currentUI {
			case MainMenu:
				m.currentUI, cmds = m.menu.Update(msg)

				if m.currentUI == Quit {
					return m, tea.Quit
				}

				if m.currentUI == Live {
					liveConnection := NewLiveConnection(m.cache)

					// No live event so do nothing
					if liveConnection == nil {
						// TODO - message for user
						m.menu.message = "There is no live session currently happening."
						m.currentUI = MainMenu
					} else {
						m.replayUI.Enter(liveConnection, m.currentUI, true)
					}
				}

				if m.currentUI == DebugReplay {
					m.replayUI.Enter(NewDebugReplayConnection(m.cache, m.debugFile), m.currentUI, false)
				}

			case Live:
				m.currentUI, cmds = m.replayUI.Update(msg)
				if m.currentUI != Live {
					m.replayUI.Leave()
					m.menu.Enter()
				}

			case Replay:
				m.currentUI, cmds = m.replayUI.Update(msg)
				if m.currentUI != Replay {
					m.replayUI.Leave()
					m.menu.Enter()
				}

			case ReplayMenu:
				m.currentUI, cmds = m.replayMenu.Update(msg)
				if m.currentUI == Replay {
					m.replayUI.Enter(NewReplayConnection(m.cache, m.replayMenu.choice.event), m.currentUI, false)
				}

			case DebugReplay:
				m.currentUI, cmds = m.replayUI.Update(msg)
				if m.currentUI != DebugReplay {
					m.replayUI.Leave()
					m.menu.Enter()
				}
			}
		}

	case tea.WindowSizeMsg:

		m.currentWidth = msg.Width
		m.currentHeight = msg.Height
		m.ready = true

		m.menu.Resize(msg)
		m.replayMenu.Resize(msg)
		m.replayUI.Resize(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m ui) View() string {
	switch m.currentUI {
	case MainMenu:
		return m.menu.View()

	case Live:
		return m.replayUI.View()

	case Replay:
		return m.replayUI.View()

	case DebugReplay:
		return m.replayUI.View()

	case ReplayMenu:
		return m.replayMenu.View()

	case Quit:

	default:
		panic("")
	}

	return ""
}
