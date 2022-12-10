package main

import (
	"fmt"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/f1gopher/f1gopherlib"
	"io"
)

type replayMenu struct {
	cursor        int
	currentWidth  int
	currentHeight int

	list   list.Model
	choice item
}

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type item struct {
	event f1gopherlib.RaceEvent
}

func (i item) FilterValue() string { return "" }

type itemDelegate struct{}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %d %s %s", index+1, i.event.RaceTime.Year(), i.event.Country, i.event.Type.String())

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s string) string {
			return selectedItemStyle.Render("> " + s)
		}
	}

	fmt.Fprint(w, fn(str))
}

func NewReplayMenu() *replayMenu {
	items := []list.Item{}

	for _, event := range f1gopherlib.RaceHistory() {
		items = append(items, item{event: event})
	}

	l := list.New(items, itemDelegate{}, 200, 20)
	l.Title = "Select a session to replay"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	l.Styles.Title = titleStyle
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle

	return &replayMenu{
		cursor: 0,
		list:   l,
	}
}

func (m *replayMenu) Resize(msg tea.WindowSizeMsg) {
	m.currentWidth = msg.Width
	m.currentHeight = msg.Height
	m.list.SetHeight(msg.Height - 1)
}

func (m *replayMenu) Update(msg tea.Msg) (newUI uiPage, cmds []tea.Cmd) {
	newUI = ReplayMenu

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter, tea.KeySpace:
			selected, ok := m.list.SelectedItem().(item)
			if ok {
				m.choice = selected
			}

			return Replay, nil

		case tea.KeyEsc:
			return MainMenu, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return newUI, []tea.Cmd{cmd}
}

func (m *replayMenu) View() string {
	return "\n" + m.list.View()
}
