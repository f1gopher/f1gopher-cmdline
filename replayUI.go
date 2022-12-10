package main

import (
	"bytes"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/f1gopher/f1gopherlib"
	"github.com/f1gopher/f1gopherlib/Messages"
	"github.com/gorilla/mux"
	"github.com/hajimehoshi/go-mp3"
	"github.com/hajimehoshi/oto/v2"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const TrendSize = 10
const timeWidth = 11

type driverTrend struct {
	data  []int64
	trend int64
}

type replayUI struct {
	tick          func() (updateContent bool)
	err           error
	ui            uiPage
	currentWidth  int
	currentHeight int

	f        f1gopherlib.F1GopherLib
	data     map[int]Messages.Timing
	dataLock sync.Mutex

	event     Messages.Event
	eventLock sync.Mutex

	rcMessages     []Messages.RaceControlMessage
	rcMessagesLock sync.Mutex

	radio     []Messages.Radio
	radioLock sync.Mutex
	radioName string

	weather     Messages.Weather
	weatherLock sync.Mutex

	eventTime     time.Time
	remainingTime time.Duration
	isMuted       bool
	gapToInfront  bool

	wg   sync.WaitGroup
	exit atomic.Bool

	fastestSector1        time.Duration
	fastestSector2        time.Duration
	fastestSector3        time.Duration
	theoreticalFastestLap time.Duration
	previousSessionActive Messages.SessionState
	fastestSpeedTrap      int

	driverGapTrend map[int]driverTrend

	servers          []string
	html             string
	liveDelay        time.Duration
	liveStartTime    time.Time
	liveDelayExpired bool
}

func NewReplayUI(servers []string, liveDelay time.Duration) *replayUI {
	abc := &replayUI{
		err:       nil,
		data:      make(map[int]Messages.Timing),
		servers:   servers,
		liveDelay: liveDelay,
	}

	go abc.webServer()

	return abc
}

func (m *replayUI) Enter(data f1gopherlib.F1GopherLib, ui uiPage, isLive bool) {
	m.exit.Store(false)
	m.f = data
	m.ui = ui
	m.fastestSector1 = 0
	m.fastestSector2 = 0
	m.fastestSector3 = 0
	m.theoreticalFastestLap = 0
	m.previousSessionActive = Messages.Inactive
	m.driverGapTrend = make(map[int]driverTrend, 0)
	m.liveDelayExpired = false

	go m.listen()
	go m.playTeamRadio()

	if isLive {
		m.liveStartTime = time.Now().Add(m.liveDelay)

		// If no delay then unpause
		if m.liveDelay == 0 {
			m.f.TogglePause()
		}

	} else {
		m.liveStartTime = time.Now()
		m.liveDelayExpired = true
	}

	m.gapToInfront = data.Session() == Messages.RaceSession || data.Session() == Messages.SprintSession
}

func (m *replayUI) Leave() {

	// TODO - tell f1data to quit
	m.exit.Store(true)
	m.wg.Wait()

	m.f = nil
	m.data = make(map[int]Messages.Timing)
	m.event = Messages.Event{}
	m.rcMessages = make([]Messages.RaceControlMessage, 0)
	m.radio = make([]Messages.Radio, 0)
	m.radioName = ""
	m.weather = Messages.Weather{}
	m.eventTime = time.Time{}
	m.remainingTime = 0
	m.driverGapTrend = make(map[int]driverTrend, 0)

	m.html = ""
}

func (m *replayUI) Update(msg tea.Msg) (newUI uiPage, cmds []tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	//case tickMsg:
	//	return m, tick()

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return MainMenu, nil

		case tea.KeyUp:
			m.f.IncrementTime(time.Minute * 1)

		case tea.KeyCtrlCloseBracket:
			m.f.IncrementTime(time.Second * 5)

		case tea.KeyRight:
			m.f.IncrementLap()

		default:
			switch msg.String() {
			case "r":
				m.isMuted = !m.isMuted

			case "t":
				m.gapToInfront = !m.gapToInfront

			case "p":
				m.f.TogglePause()

			case "s":
				m.f.SkipToSessionStart()
			}
		}

	}

	cmds = append(cmds, cmd)
	return m.ui, cmds
}

func (m *replayUI) Resize(msg tea.WindowSizeMsg) {
	m.currentWidth = msg.Width
	m.currentHeight = msg.Height
}

func (m *replayUI) View() string {
	if m.liveStartTime.After(time.Now()) {
		return lipgloss.Place(m.currentWidth, m.currentHeight, lipgloss.Center, lipgloss.Center,
			fmt.Sprintf("Delaying start, %.1f seconds...", m.liveStartTime.Sub(time.Now()).Seconds()))
	} else if !m.liveDelayExpired {
		m.liveDelayExpired = true
		// Unpause data
		if m.f.IsPaused() {
			m.f.TogglePause()
		}
	}

	hour := int(m.remainingTime.Seconds() / 3600)
	minute := int(m.remainingTime.Seconds()/60) % 60
	second := int(m.remainingTime.Seconds()) % 60
	remaining := fmt.Sprintf("%d:%02d:%02d", hour, minute, second)
	var table string
	var seperator = ""

	v := make([]Messages.Timing, 0)

	m.dataLock.Lock()
	for _, a := range m.data {
		v = append(v, a)
	}
	m.dataLock.Unlock()

	sort.Slice(v, func(i, j int) bool {
		return v[i].Position < v[j].Position
	})

	segmentCount := m.event.TotalSegments
	if segmentCount == 0 {
		segmentCount = len("Segment")
	}

	// Track the fastest sectors times for the session
	for _, driver := range v {
		if (driver.Sector1 > 0 && driver.Sector1 < m.fastestSector1) || m.fastestSector1 == 0 {
			m.fastestSector1 = driver.Sector1
		}

		if (driver.Sector2 > 0 && driver.Sector2 < m.fastestSector2) || m.fastestSector2 == 0 {
			m.fastestSector2 = driver.Sector2
		}

		if (driver.Sector3 > 0 && driver.Sector3 < m.fastestSector3) || m.fastestSector3 == 0 {
			m.fastestSector3 = driver.Sector3
		}

		if driver.SpeedTrap > m.fastestSpeedTrap {
			m.fastestSpeedTrap = driver.SpeedTrap
		}
	}

	if m.fastestSector1 > 0 && m.fastestSector2 > 0 && m.fastestSector3 > 0 {
		m.theoreticalFastestLap = m.fastestSector1 + m.fastestSector2 + m.fastestSector3
	}

	if m.event.Status == Messages.Started {
		if m.previousSessionActive != Messages.Started {
			m.fastestSector1 = 0
			m.fastestSector2 = 0
			m.fastestSector3 = 0
			m.theoreticalFastestLap = 0
			m.previousSessionActive = m.event.Status
		}
	} else if m.event.Status == Messages.Inactive {
		m.fastestSector1 = 0
		m.fastestSector2 = 0
		m.fastestSector3 = 0
		m.theoreticalFastestLap = 0
		m.previousSessionActive = m.event.Status
	} else {
		m.previousSessionActive = m.event.Status
	}

	if m.f.Session() == Messages.RaceSession || m.f.Session() == Messages.SprintSession {
		seperator = "---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------"

		title := fmt.Sprintf("%s: %v, Track Time: %v, Status: %s, DRS: %v, Safety Car: %s, Lap: %d/%d, Remaining: %s %s\n",
			m.f.Name(),
			m.event.Type.String(),
			m.eventTime.In(m.f.CircuitTimezone()).Format("2006-01-02 15:04:05"),
			lipgloss.NewStyle().Foreground(lipgloss.Color(trackStatusColor(m.event.TrackStatus))).Render(m.event.TrackStatus.String()),
			m.event.DRSEnabled.String(),
			lipgloss.NewStyle().Foreground(lipgloss.Color(safetyCarFormat(m.event.SafetyCar))).Render(m.event.SafetyCar.String()),
			m.event.CurrentLap,
			m.event.TotalLaps,
			remaining,
			lipgloss.NewStyle().Foreground(lipgloss.Color(trackStatusColor(m.event.TrackStatus))).Render("⚑"))

		header := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render("Pos"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render("Driver"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(segmentCount+2).Padding(0, 1, 0, 1).Render("Segment"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Fastest"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Gap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S1"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S2"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S3"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Last Lap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render("DRS"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render("Tire"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render("Lap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render("Pitstops"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Render("Speed Trap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(13).Padding(0, 1, 0, 1).Render("Location"))

		table = title + header + "\n" + seperator + "\n"

		for _, driver := range v {

			if driver.Location == Messages.Stopped {
				row := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(driver.Color)).Render(driver.ShortName),
					lipgloss.NewStyle().Align(lipgloss.Left).Width(segmentCount+2).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(fastestLapColor(driver.OverallFastestLap))).Render(fmtDuration(driver.FastestLap)),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(13).Padding(0, 1, 0, 1).Render(driver.Location.String()))
				table += row + "\n"
				continue
			}

			speedTrap := ""
			if driver.SpeedTrap > 0 {
				speedTrap = fmt.Sprintf("%d", driver.SpeedTrap)
			}

			gap := driver.GapToLeader
			if m.gapToInfront {
				gap = driver.TimeDiffToPositionAhead
			}
			drs := "Closed"
			if driver.DRSOpen {
				drs = "Open"
			}

			segments := ""
			for x := 0; x < segmentCount; x++ {
				switch driver.Segment[x] {
				case Messages.None:
					segments += " "
				default:
					segments += lipgloss.NewStyle().Foreground(segmentColor(driver.Segment[x])).Render("■")
				}

				if x == m.event.Sector1Segments-1 || x == m.event.Sector1Segments+m.event.Sector2Segments-1 {
					segments += "|"
				}
			}

			gapColor := lipgloss.Color("#FFFFFF")
			trend, exists := m.driverGapTrend[driver.Number]
			if exists {
				if trend.trend > 0 {
					if trend.trend > 10 {
						gapColor = "#FF0000"
					}

				} else if trend.trend < 0 {
					if trend.trend < -10 {
						gapColor = "#00FF00"
					}
				}
			}

			drsColor := lipgloss.Color("#FFFFFF")
			if driver.TimeDiffToPositionAhead > 0 && driver.TimeDiffToPositionAhead < time.Second {
				drsColor = "#00FF00"
			}

			row := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
				lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(driver.Color)).Render(driver.ShortName),
				lipgloss.NewStyle().Align(lipgloss.Left).Width(m.event.Sector1Segments+m.event.Sector2Segments+m.event.Sector3Segments+2).Render(segments),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(fastestLapColor(driver.OverallFastestLap))).Render(fmtDuration(driver.FastestLap)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(gapColor).Render(fmtDuration(gap)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.Sector1PersonalFastest, driver.Sector1OverallFastest))).Render(fmtDuration(driver.Sector1)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.Sector2PersonalFastest, driver.Sector2OverallFastest))).Render(fmtDuration(driver.Sector2)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.Sector3PersonalFastest, driver.Sector3OverallFastest))).Render(fmtDuration(driver.Sector3)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.LastLapPersonalFastest, driver.LastLapOverallFastest))).Render(fmtDuration(driver.LastLap)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Foreground(drsColor).Render(drs),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(tireColor(driver.Tire))).Render(driver.Tire.String()),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.LapsOnTire)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Pitstops)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.SpeedTrapPersonalFastest, driver.SpeedTrapOverallFastest))).Render(speedTrap),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(13).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(locationColor(driver.Location))).Render(driver.Location.String()))

			if driver.ChequeredFlag {
				row = row + " 🏁"
			}

			table += row + "\n"
		}

	} else {
		seperator = "--------------------------------------------------------------------------------------------------------------------------------------------------------------"

		title := fmt.Sprintf("%s: %v, Track Time: %v, Status: %s, DRS: %s, Remaining: %s %s\n",
			m.f.Name(),
			m.event.Type.String(),
			m.eventTime.In(m.f.CircuitTimezone()).Format("2006-01-02 15:04:05"),
			lipgloss.NewStyle().Foreground(lipgloss.Color(sessionStatusColor(m.event.Status))).Render(m.event.Status.String()),
			m.event.DRSEnabled.String(),
			remaining,
			lipgloss.NewStyle().Foreground(lipgloss.Color(trackStatusColor(m.event.TrackStatus))).Render("⚑"))

		header := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render("Pos"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render("Driver"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(segmentCount+2).Padding(0, 1, 0, 1).Render("Segment"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Fastest"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Gap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S1"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S2"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S3"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Last Lap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render("Tire"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render("Lap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Render("Speed Trap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(13).Padding(0, 1, 0, 1).Render("Location"))

		table = title + header + "\n" + seperator + "\n"

		outBackground := lipgloss.AdaptiveColor{Light: "#4545E4", Dark: "#4545E4"}
		dropZoneBackground := lipgloss.AdaptiveColor{Light: "#53544E", Dark: "#53544E"}

		for x, driver := range v {
			speedTrap := ""
			if driver.SpeedTrap > 0 {
				speedTrap = fmt.Sprintf("%d", driver.SpeedTrap)
			}

			gap := driver.TimeDiffToFastest
			if m.gapToInfront {
				gap = driver.TimeDiffToPositionAhead
			}

			segments := ""
			for x := 0; x < segmentCount; x++ {
				switch driver.Segment[x] {
				case Messages.None:
					segments += " "
				default:
					segments += lipgloss.NewStyle().Foreground(segmentColor(driver.Segment[x])).Render("■")
				}

				if x == m.event.Sector1Segments-1 || x == m.event.Sector1Segments+m.event.Sector2Segments-1 {
					segments += "|"
				}
			}

			var row string
			if !driver.KnockedOutOfQualifying {

				if m.event.Type == Messages.Qualifying1 && x >= 15 ||
					m.event.Type == Messages.Qualifying2 && x >= 10 {

					row = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
						lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Background(dropZoneBackground).Render(fmt.Sprintf("%d", driver.Position)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(driver.Color)).Render(driver.ShortName),
						lipgloss.NewStyle().Align(lipgloss.Left).Width(segmentCount+2).Render(segments),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(dropZoneBackground).Render(fmtDuration(driver.FastestLap)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(dropZoneBackground).Render(fmtDuration(gap)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(dropZoneBackground).Foreground(lipgloss.Color(timeColor(driver.Sector1PersonalFastest, driver.Sector1OverallFastest))).Render(fmtDuration(driver.Sector1)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(dropZoneBackground).Foreground(lipgloss.Color(timeColor(driver.Sector2PersonalFastest, driver.Sector2OverallFastest))).Render(fmtDuration(driver.Sector2)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(dropZoneBackground).Foreground(lipgloss.Color(timeColor(driver.Sector3PersonalFastest, driver.Sector3OverallFastest))).Render(fmtDuration(driver.Sector3)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(dropZoneBackground).Foreground(lipgloss.Color(timeColor(driver.LastLapPersonalFastest, driver.LastLapOverallFastest))).Render(fmtDuration(driver.LastLap)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Background(dropZoneBackground).Foreground(lipgloss.Color(tireColor(driver.Tire))).Render(driver.Tire.String()),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Background(dropZoneBackground).Render(fmt.Sprintf("%d", driver.LapsOnTire)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Background(dropZoneBackground).Foreground(lipgloss.Color(timeColor(driver.SpeedTrapPersonalFastest, driver.SpeedTrapOverallFastest))).Render(speedTrap),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(13).Padding(0, 1, 0, 1).Background(dropZoneBackground).Foreground(lipgloss.Color(locationColor(driver.Location))).Render(driver.Location.String()))
				} else {
					row = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
						lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(driver.Color)).Render(driver.ShortName),
						lipgloss.NewStyle().Align(lipgloss.Left).Width(segmentCount+2).Render(segments),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.FastestLap)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(gap)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.Sector1PersonalFastest, driver.Sector1OverallFastest))).Render(fmtDuration(driver.Sector1)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.Sector2PersonalFastest, driver.Sector2OverallFastest))).Render(fmtDuration(driver.Sector2)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.Sector3PersonalFastest, driver.Sector3OverallFastest))).Render(fmtDuration(driver.Sector3)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.LastLapPersonalFastest, driver.LastLapOverallFastest))).Render(fmtDuration(driver.LastLap)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(tireColor(driver.Tire))).Render(driver.Tire.String()),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.LapsOnTire)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(timeColor(driver.SpeedTrapPersonalFastest, driver.SpeedTrapOverallFastest))).Render(speedTrap),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(13).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(locationColor(driver.Location))).Render(driver.Location.String()))
				}

				if driver.ChequeredFlag {
					row = row + " 🏁"
				}

			} else {

				row = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Background(outBackground).Render(fmt.Sprintf("%d", driver.Position)),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(driver.Color)).Render(driver.ShortName),
					lipgloss.NewStyle().Align(lipgloss.Left).Width(segmentCount+2).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(outBackground).Render(fmtDuration(driver.FastestLap)),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Background(outBackground).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(13).Padding(0, 1, 0, 1).Background(outBackground).Render("Out"))
			}

			table += row + "\n"
		}

	}

	table += seperator + "\n"
	trackStatus := "Track Status: |"
	for x := 0; x < segmentCount; x++ {
		switch m.event.SegmentFlags[x] {
		case Messages.GreenFlag:
			trackStatus += lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("■")
		case Messages.YellowFlag:
			trackStatus += lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render("■")
		case Messages.DoubleYellowFlag:
			trackStatus += lipgloss.NewStyle().Foreground(lipgloss.Color("#FBFF00")).Render("■")
		case Messages.RedFlag:
			trackStatus += lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("■")
		}
		if x == m.event.Sector1Segments-1 || x == m.event.Sector1Segments+m.event.Sector2Segments-1 {
			trackStatus += "|"
		}
	}
	if m.f.Session() == Messages.RaceSession || m.f.Session() == Messages.SprintSession {
		trackStatus += fmt.Sprintf("|                       |%s|%s|%s|%s|                                    |%s|",
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(m.fastestSector1)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(m.fastestSector2)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(m.fastestSector3)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(m.theoreticalFastestLap)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmt.Sprintf("%d", m.fastestSpeedTrap)))
	} else {
		trackStatus += fmt.Sprintf("|                       |%s|%s|%s|%s|                |%s|",
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(m.fastestSector1)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(m.fastestSector2)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(m.fastestSector3)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(m.theoreticalFastestLap)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmt.Sprintf("%d", m.fastestSpeedTrap)))
	}

	table += trackStatus + "\n"

	table += seperator + "\n"
	m.rcMessagesLock.Lock()
	if len(m.rcMessages) > 0 {
		for x := len(m.rcMessages) - 1; x >= 0 && x >= len(m.rcMessages)-5; x-- {
			lastMessage := m.rcMessages[x]
			prefix := ""

			switch lastMessage.Flag {
			case Messages.ChequeredFlag:
				prefix = "🏁 "
			case Messages.GreenFlag:
				if strings.HasPrefix(lastMessage.Msg, "GREEN LIGHT") {
					prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("● ")
				} else {
					prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("⚑ ")
				}
			case Messages.YellowFlag:
				prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render("⚑ ")
			case Messages.DoubleYellowFlag:
				prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render("⚑⚑ ")
			case Messages.BlueFlag:
				prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF")).Render("⚑ ")
			case Messages.RedFlag:
				if strings.HasPrefix(lastMessage.Msg, "RED LIGHT") {
					prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("● ")
				} else {
					prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("⚑ ")
				}
			case Messages.BlackAndWhite:
				prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Render("⚑") +
					lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Render("⚑ ")
			}

			table += fmt.Sprintf("%s - %s%s\n", lastMessage.Timestamp.In(m.f.CircuitTimezone()).Format("02-01-2006 15:04:05"), prefix, lastMessage.Msg)
		}
	}
	m.rcMessagesLock.Unlock()

	status := seperator + "\n"

	m.weatherLock.Lock()
	status += fmt.Sprintf("Air Temp: %.2f°C, Track Temp: %.2f°C, ", m.weather.AirTemp, m.weather.TrackTemp)
	if m.weather.Rainfall {
		status += lipgloss.NewStyle().Foreground(lipgloss.Color("#009DD3")).Render("Raining, ")
	}
	m.weatherLock.Unlock()

	if !m.isMuted {
		status += fmt.Sprintf("Team Radio: On")
	} else {
		status += fmt.Sprintf("Team Radio: Off")
	}

	if m.radioName != "" {
		status += fmt.Sprintf(", Radio: %s", m.radioName)
	}

	// If it is a race and the session hasn't started yet (remaining time count down hasn't started) then
	// display a count down to the start of the session
	if m.f.Session() == Messages.RaceSession || m.f.Session() == Messages.SprintSession && m.remainingTime == 0 {
		status += lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(
			fmt.Sprintf(", Session Starts in: %s", fmtDuration(m.f.SessionStart().Sub(m.eventTime))))
	}

	if m.f.IsPaused() {
		status += fmt.Sprintf(", ** PAUSED **")
	}

	table += status

	m.updateHTML(v)

	return table
}

func (m *replayUI) listen() {
	m.wg.Add(1)

	for !m.exit.Load() {
		select {
		case msg2 := <-m.f.Timing():
			m.dataLock.Lock()
			m.data[msg2.Number] = msg2
			m.dataLock.Unlock()

			// For races calculate the gap to the car in  front trend
			if m.f.Session() == Messages.RaceSession || m.f.Session() == Messages.SprintSession {
				for x := range m.data {
					gap := m.data[x].TimeDiffToPositionAhead.Milliseconds()

					driverData, exists := m.driverGapTrend[m.data[x].Number]
					if !exists {
						m.driverGapTrend[m.data[x].Number] = driverTrend{data: []int64{gap}, trend: 0}
						continue
					}

					if driverData.data[len(driverData.data)-1] != gap {
						driverData.data = append(driverData.data, gap)

						if len(driverData.data) > TrendSize {
							driverData.data = driverData.data[len(driverData.data)-TrendSize:]
						}

						count := int64(len(driverData.data))
						var totalA, totalB, totalC, totalD int64 = 0, 0, 0, 0
						for y := range driverData.data {
							c := driverData.data[y] * int64(y+1)
							d := int64((y + 1) * (y + 1))

							totalA += int64(y + 1)
							totalB += driverData.data[y]
							totalC += c
							totalD += d
						}

						driverData.trend = (count*totalC - totalA*totalB) / (count*totalD - (totalA * totalA))

						m.driverGapTrend[m.data[x].Number] = driverData
					}
				}
			}

		case msg := <-m.f.Event():
			m.eventLock.Lock()
			m.event = msg
			m.eventLock.Unlock()

		case msg3 := <-m.f.Time():
			m.eventTime = msg3.Timestamp
			m.remainingTime = msg3.Remaining

		case msg4 := <-m.f.RaceControlMessages():
			m.rcMessagesLock.Lock()
			m.rcMessages = append(m.rcMessages, msg4)
			m.rcMessagesLock.Unlock()

		case msg5 := <-m.f.Radio():
			m.radioLock.Lock()
			m.radio = append(m.radio, msg5)
			m.radioLock.Unlock()

		case msg6 := <-m.f.Weather():
			m.weatherLock.Lock()
			m.weather = msg6
			m.weatherLock.Unlock()
		}
	}

	m.wg.Done()
}

func (m *replayUI) playTeamRadio() {
	m.wg.Add(1)

	c, ready, err := oto.NewContext(48000, 2, 2)
	if err != nil {
		panic(err)
	}
	<-ready

	for !m.exit.Load() {

		if len(m.radio) > 0 {
			m.radioLock.Lock()
			currentMsg := m.radio[0]
			m.radio = m.radio[1:]
			m.radioLock.Unlock()

			if !m.isMuted {
				if m.play(currentMsg, c) {
					return
				}
			}
		}

		time.Sleep(time.Second * 1)
	}

	m.wg.Done()
}

func (m *replayUI) play(currentMsg Messages.Radio, c *oto.Context) bool {
	defer func() {
		if r := recover(); r != nil {
			//fmt.Println("Recovered in f", r)
		}
	}()

	d, err := mp3.NewDecoder(bytes.NewReader(currentMsg.Msg))
	if err != nil {
		//panic(err)
		return true
	}

	m.radioName = currentMsg.Driver

	p := c.NewPlayer(d)
	defer p.Close()
	p.Play()

	for {
		time.Sleep(time.Second)
		if !p.IsPlaying() {
			break
		}
	}

	m.radioName = ""
	return false
}

func (m *replayUI) webServer() {
	router := mux.NewRouter()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		abc := `<html>
<title>GopherF1</title>
<head>    
	<meta charset="utf-8">
</head>
<script language="javascript">	
	async function subscribe() {
  		let response = await fetch("/data");
	
		let message = await response.text();
		document.getElementById("display").innerHTML = message;
		
		await new Promise(resolve => setTimeout(resolve, 1000));
		await subscribe();
  	}

	subscribe();

</script>
<body style="background-color:black; color:white">
	<div>
		<pre id="display"></pre>
	</div>
</body>
</html>`

		w.Write([]byte(abc))
	})

	router.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(m.html))
	})

	for x := range m.servers {

		srv := &http.Server{
			Handler: router,
			Addr:    m.servers[x],
			// Good practice: enforce timeouts for servers you create!
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		go srv.ListenAndServe()
	}
}

func (m *replayUI) updateHTML(v []Messages.Timing) {
	hour := int(m.remainingTime.Seconds() / 3600)
	minute := int(m.remainingTime.Seconds()/60) % 60
	second := int(m.remainingTime.Seconds()) % 60
	remaining := fmt.Sprintf("%d:%02d:%02d", hour, minute, second)
	segmentCount := m.event.TotalSegments
	if segmentCount == 0 {
		segmentCount = len("Segment")
	}
	var table string
	var seperator = ""

	if m.f.Session() == Messages.RaceSession || m.f.Session() == Messages.SprintSession {

		seperator = "---------------------------------------------------------------------------------------------------------------------------------------------"

		title := fmt.Sprintf("%s: %v, Track Time: %v, Status: %s, DRS: %v, Safety Car: %s, Lap: %d/%d, Remaining: %s %s\n",
			m.f.Name(),
			m.event.Type.String(),
			m.eventTime.In(m.f.CircuitTimezone()).Format("2006-01-02 15:04:05"),
			fmt.Sprintf("<font color=\"%s\">%s</font>", sessionStatusColor(m.event.Status), m.event.Status.String()),
			m.event.DRSEnabled.String(),
			fmt.Sprintf("<font color=\"%s\">%s</font>", safetyCarFormat(m.event.SafetyCar), m.event.SafetyCar),
			m.event.CurrentLap,
			m.event.TotalLaps,
			remaining,
			fmt.Sprintf("<font color=\"%s\">&#x2691</font>", trackStatusColor(m.event.TrackStatus)))

		header := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render("Pos"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render("Driver"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(segmentCount+2).Padding(0, 1, 0, 1).Render("Segment"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render("Fastest"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render("Gap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render("S1"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render("S2"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render("S3"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render("Last Lap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(6).Render("DRS"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Render("Tire"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(3).Render("Lap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(4).Render("Pits"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render("Speed"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Render("Location"))

		table = title + header + "\n" + seperator + "\n"

		for _, driver := range v {
			if driver.Location == Messages.Stopped {
				row := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
					fmt.Sprintf("<font color=\"%s\">%s</font>", driver.Color, lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render(driver.ShortName)),
					lipgloss.NewStyle().Align(lipgloss.Left).Width(segmentCount+2).Render(""),
					fmt.Sprintf("<font color=\"%s\">%s</font>", fastestLapColor(driver.OverallFastestLap), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.FastestLap))),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(6).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(3).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(4).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(""),
					fmt.Sprintf("<font color=\"%s\">%s</font>", locationColor(driver.Location), lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Render(driver.Location.String())))
				table += row + "\n"
				continue
			}

			speedTrap := ""
			if driver.SpeedTrap > 0 {
				speedTrap = fmt.Sprintf("%d", driver.SpeedTrap)
			}

			gap := driver.GapToLeader
			if m.gapToInfront {
				gap = driver.TimeDiffToPositionAhead
			}
			drs := "Closed"
			if driver.DRSOpen {
				drs = "Open"
			}

			segments := ""
			for x := 0; x < segmentCount; x++ {
				switch driver.Segment[x] {
				case Messages.None:
					segments += " "
				default:
					segments += fmt.Sprintf("<font color=\"%s\">&#x25a0;</font>", segmentColor(driver.Segment[x]))
				}

				if x == m.event.Sector1Segments-1 || x == m.event.Sector1Segments+m.event.Sector2Segments-1 {
					segments += "|"
				}
			}

			gapColor := lipgloss.Color("#FFFFFF")
			trend, exists := m.driverGapTrend[driver.Number]
			if exists {
				if trend.trend > 0 {
					if trend.trend > 10 {
						gapColor = "#FF0000"
					}

				} else if trend.trend < 0 {
					if trend.trend < -10 {
						gapColor = "#00FF00"
					}
				}
			}

			drsColor := lipgloss.Color("#FFFFFF")
			if driver.TimeDiffToPositionAhead > 0 && driver.TimeDiffToPositionAhead < time.Second {
				drsColor = "#00FF00"
			}

			row := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
				lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
				fmt.Sprintf("<font color=\"%s\">%s</font>", driver.Color, lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render(driver.ShortName)),
				segments,
				fmt.Sprintf("<font color=\"%s\">%s</font>", fastestLapColor(driver.OverallFastestLap), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(driver.FastestLap))),
				fmt.Sprintf("<font color=\"%s\">%s</font>", gapColor, lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(gap))),
				fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector1PersonalFastest, driver.Sector1OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(driver.Sector1))),
				fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector2PersonalFastest, driver.Sector2OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(driver.Sector2))),
				fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector3PersonalFastest, driver.Sector3OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(driver.Sector3))),
				fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.LastLapPersonalFastest, driver.LastLapOverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(driver.LastLap))),
				fmt.Sprintf("<font color=\"%s\">%s</font>", drsColor, lipgloss.NewStyle().Align(lipgloss.Center).Width(6).Render(drs)),
				fmt.Sprintf("<font color=\"%s\">%s</font>", tireColor(driver.Tire), lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Render(driver.Tire.String())),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(3).Render(fmt.Sprintf("%d", driver.LapsOnTire)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(4).Render(fmt.Sprintf("%d", driver.Pitstops)),
				fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.SpeedTrapPersonalFastest, driver.SpeedTrapOverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(speedTrap)),
				fmt.Sprintf("<font color=\"%s\">%s</font>", locationColor(driver.Location), lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Render(driver.Location.String())))

			if driver.ChequeredFlag {
				row = row + " 🏁"
			}

			table += row + "\n"
		}
	} else {
		seperator = "------------------------------------------------------------------------------------------------------------------------------------------------------"

		title := fmt.Sprintf("%s: %v, Track Time: %v, Status: %s, DRS: %s, Remaining: %s %s\n",
			m.f.Name(),
			m.event.Type.String(),
			m.eventTime.In(m.f.CircuitTimezone()).Format("2006-01-02 15:04:05"),
			fmt.Sprintf("<font color=\"%s\">%s</font>", sessionStatusColor(m.event.Status), m.event.Status.String()),
			m.event.DRSEnabled.String(),
			remaining,
			fmt.Sprintf("<font color=\"%s\">&#x2691</font>", trackStatusColor(m.event.TrackStatus)))

		header := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render("Pos"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render("Driver"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(segmentCount+2).Padding(0, 1, 0, 1).Render("Segment"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Fastest"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Gap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S1"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S2"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("S3"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render("Last Lap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render("Tire"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render("Lap"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render("Speed"),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render("Location"))

		table = title + header + "\n" + seperator + "\n"

		outBackground := "#4545E4"
		dropZoneBackground := "#53544E"

		for x, driver := range v {
			speedTrap := ""
			if driver.SpeedTrap > 0 {
				speedTrap = fmt.Sprintf("%d", driver.SpeedTrap)
			}

			gap := driver.TimeDiffToFastest
			if m.gapToInfront {
				gap = driver.TimeDiffToPositionAhead
			}

			segments := ""
			for x := 0; x < segmentCount; x++ {
				switch driver.Segment[x] {
				case Messages.None:
					segments += " "
				default:
					segments += fmt.Sprintf("<font color=\"%s\">&#x25a0;</font>", segmentColor(driver.Segment[x]))
				}

				if x == m.event.Sector1Segments-1 || x == m.event.Sector1Segments+m.event.Sector2Segments-1 {
					segments += "|"
				}
			}

			var row string
			if !driver.KnockedOutOfQualifying {

				if m.event.Type == Messages.Qualifying1 && x >= 15 ||
					m.event.Type == Messages.Qualifying2 && x >= 10 {

					row = fmt.Sprintf("<pr style=\"background-color: %s\">%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s</pr>",
						dropZoneBackground,
						lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
						fmt.Sprintf("<font color=\"%s\">%s</font>", driver.Color, lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render(driver.ShortName)),
						segments,
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.FastestLap)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(gap)),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector1PersonalFastest, driver.Sector1OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.Sector1))),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector2PersonalFastest, driver.Sector2OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.Sector2))),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector3PersonalFastest, driver.Sector3OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.Sector3))),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.LastLapPersonalFastest, driver.LastLapOverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.LastLap))),
						fmt.Sprintf("<font color=\"%s\">%s</font>", tireColor(driver.Tire), lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render(driver.Tire.String())),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.LapsOnTire)),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.SpeedTrapPersonalFastest, driver.SpeedTrapOverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(speedTrap)),
						fmt.Sprintf("<font color=\"%s\">%s</font>", locationColor(driver.Location), lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render(driver.Location.String())))

				} else {
					row = fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
						lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
						fmt.Sprintf("<font color=\"%s\">%s</font>", driver.Color, lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render(driver.ShortName)),
						segments,
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.FastestLap)),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(gap)),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector1PersonalFastest, driver.Sector1OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.Sector1))),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector2PersonalFastest, driver.Sector2OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.Sector2))),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.Sector3PersonalFastest, driver.Sector3OverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.Sector3))),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.LastLapPersonalFastest, driver.LastLapOverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.LastLap))),
						fmt.Sprintf("<font color=\"%s\">%s</font>", tireColor(driver.Tire), lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render(driver.Tire.String())),
						lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.LapsOnTire)),
						fmt.Sprintf("<font color=\"%s\">%s</font>", timeColor(driver.SpeedTrapPersonalFastest, driver.SpeedTrapOverallFastest), lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(speedTrap)),
						fmt.Sprintf("<font color=\"%s\">%s</font>", locationColor(driver.Location), lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render(driver.Location.String())))
				}

				if driver.ChequeredFlag {
					row = row + " 🏁"
				}

			} else {

				row = fmt.Sprintf("<pr style=\"background-color: %s\">%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s</pr>",
					outBackground,
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
					fmt.Sprintf("<font color=\"%s\">%s</font>", driver.Color, lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Render(driver.ShortName)),
					lipgloss.NewStyle().Align(lipgloss.Left).Width(segmentCount+2).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(driver.FastestLap)),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(""),
					lipgloss.NewStyle().Align(lipgloss.Center).Width(10).Padding(0, 1, 0, 1).Render("Out"))
			}

			table += row + "\n"
		}
	}

	table += seperator + "\n"
	trackStatus := "Track Status: |"
	for x := 0; x < segmentCount; x++ {
		switch m.event.SegmentFlags[x] {
		case Messages.GreenFlag:
			trackStatus += "<font color=\"#00FF00\">&#x25a0;</font>"
		case Messages.YellowFlag:
			trackStatus += "<font color=\"#FFFF00\">&#x25a0;</font>"
		case Messages.DoubleYellowFlag:
			trackStatus += "<font color=\"#FBFF00\">&#x25a0;</font>"
		case Messages.RedFlag:
			trackStatus += "<font color=\"#FF0000\">&#x25a0;</font>"
		}
		if x == m.event.Sector1Segments-1 || x == m.event.Sector1Segments+m.event.Sector2Segments-1 {
			trackStatus += "|"
		}
	}
	if m.f.Session() == Messages.RaceSession || m.f.Session() == Messages.SprintSession {
		trackStatus += fmt.Sprintf("|                   |%s|%s|%s|%s|                        |%s|",
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(m.fastestSector1))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(m.fastestSector2))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(m.fastestSector3))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(m.theoreticalFastestLap))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(fmt.Sprintf("%d", m.fastestSpeedTrap))))
	} else {
		trackStatus += fmt.Sprintf("|                       |%s|%s|%s|%s|                |%s|",
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(m.fastestSector1))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(m.fastestSector2))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(m.fastestSector3))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(m.theoreticalFastestLap))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(fmt.Sprintf("%d", m.fastestSpeedTrap))))
	}

	table += trackStatus + "\n"

	table += seperator + "\n"
	m.rcMessagesLock.Lock()
	if len(m.rcMessages) > 0 {
		for x := len(m.rcMessages) - 1; x >= 0 && x >= len(m.rcMessages)-19; x-- {
			lastMessage := m.rcMessages[x]
			prefix := ""

			switch lastMessage.Flag {
			case Messages.ChequeredFlag:
				prefix = "🏁 "
			case Messages.GreenFlag:
				if strings.HasPrefix(lastMessage.Msg, "GREEN LIGHT") {
					prefix = "<font color=\"#00FF00\">&#11044; </font>"
				} else {
					prefix = "<font color=\"#00FF00\">&#x2691; </font>"
				}
			case Messages.YellowFlag:
				prefix = "<font color=\"#FFFF00\">&#x2691; </font>"
			case Messages.DoubleYellowFlag:
				prefix = "<font color=\"#FFFF00\">&#x2691;&#x2691; </font>"
			case Messages.BlueFlag:
				prefix = "<font color=\"#0000FF\">&#x2691; </font>"
			case Messages.RedFlag:
				if strings.HasPrefix(lastMessage.Msg, "RED LIGHT") {
					prefix = "<font color=\"#FF0000\">&#11044; </font>"
				} else {
					prefix = "<font color=\"#FF0000\">&#x2691; </font>"
				}
			case Messages.BlackAndWhite:
				prefix = "<font color=\"#000000\">&#x2691;</font>" + "<font color=\"#FFFFFF\">&#x2691; </font>"
			}

			table += fmt.Sprintf("%s - %s%s\n", lastMessage.Timestamp.In(m.f.CircuitTimezone()).Format("02-01-2006 15:04:05"), prefix, lastMessage.Msg)
		}
	}
	m.rcMessagesLock.Unlock()

	status := seperator + "\n"

	m.weatherLock.Lock()
	status += fmt.Sprintf("Air Temp: %.2f°C, Track Temp: %.2f°C", m.weather.AirTemp, m.weather.TrackTemp)
	if m.weather.Rainfall {
		status += ", <font color=\"#009DD3\">Raining</font>"
	}
	m.weatherLock.Unlock()

	// If it is a race and the session hasn't started yet (remaining time count down hasn't started) then
	// display a count down to the start of the session
	if m.f.Session() == Messages.RaceSession || m.f.Session() == Messages.SprintSession && m.remainingTime == 0 {
		status += lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(
			fmt.Sprintf(", <font color=\"#00FF00\">Session Starts in: %s</font>", fmtDuration(m.f.SessionStart().Sub(m.eventTime))))
	}

	table += status

	m.html = table
}
