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

package sessionUI

import (
	"bytes"
	"f1gopher/f1gopher-cmdline/ui"
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

type sessionBase struct {
	tick          func() (updateContent bool)
	err           error
	ui            ui.Page
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
	driverGapLock  sync.Mutex

	servers          []string
	html             string
	liveDelay        time.Duration
	liveStartTime    time.Time
	liveDelayExpired bool

	renderDataForScreen func(segmentCount int, remaining string, v []Messages.Timing) (table string, separator string)
	renderDataForHtml   func(segmentCount int, remaining string, v []Messages.Timing) (table string, separator string)
}

func (s *sessionBase) Enter(data f1gopherlib.F1GopherLib, ui ui.Page, isLive bool) {
	s.exit.Store(false)
	s.f = data
	s.ui = ui
	s.fastestSector1 = 0
	s.fastestSector2 = 0
	s.fastestSector3 = 0
	s.theoreticalFastestLap = 0
	s.previousSessionActive = Messages.Inactive
	s.driverGapTrend = make(map[int]driverTrend, 0)
	s.liveDelayExpired = false

	go s.listen()
	go s.playTeamRadio()

	if isLive {
		s.liveStartTime = time.Now().Add(s.liveDelay)

		// If no delay then unpause
		if s.liveDelay == 0 {
			s.f.TogglePause()
		}

	} else {
		s.liveStartTime = time.Now()
		s.liveDelayExpired = true
	}

	s.gapToInfront = data.Session() == Messages.RaceSession || data.Session() == Messages.SprintSession
}

func (s *sessionBase) Leave() {

	// TODO - tell f1data to quit
	s.exit.Store(true)
	s.wg.Wait()

	s.f = nil
	s.data = make(map[int]Messages.Timing)
	s.event = Messages.Event{}
	s.rcMessages = make([]Messages.RaceControlMessage, 0)
	s.radio = make([]Messages.Radio, 0)
	s.radioName = ""
	s.weather = Messages.Weather{}
	s.eventTime = time.Time{}
	s.remainingTime = 0
	s.driverGapTrend = make(map[int]driverTrend, 0)

	s.html = ""
}

func (s *sessionBase) Update(msg tea.Msg) (newUI ui.Page, cmds []tea.Cmd) {
	var cmd tea.Cmd

	switch msgType := msg.(type) {
	case tea.KeyMsg:
		switch msgType.Type {
		case tea.KeyEsc:
			return ui.MainMenu, nil

		case tea.KeyUp:
			s.f.IncrementTime(time.Minute * 1)

		case tea.KeyCtrlCloseBracket:
			s.f.IncrementTime(time.Second * 5)

		case tea.KeyRight:
			s.f.IncrementLap()

		default:
			switch msgType.String() {
			case "r":
				s.isMuted = !s.isMuted

			case "t":
				s.gapToInfront = !s.gapToInfront

			case "p":
				s.f.TogglePause()

			case "s":
				s.f.SkipToSessionStart()
			}
		}

	}

	cmds = append(cmds, cmd)
	return s.ui, cmds
}

func (s *sessionBase) Resize(msg tea.WindowSizeMsg) {
	s.currentWidth = msg.Width
	s.currentHeight = msg.Height
}

func (s *sessionBase) listen() {
	s.wg.Add(1)

	for !s.exit.Load() {
		select {
		case msg2 := <-s.f.Timing():
			s.dataLock.Lock()
			s.data[msg2.Number] = msg2
			s.dataLock.Unlock()

			// For races calculate the gap to the car in  front trend
			if s.f.Session() == Messages.RaceSession || s.f.Session() == Messages.SprintSession {
				s.driverGapLock.Lock()
				for x := range s.data {
					gap := s.data[x].TimeDiffToPositionAhead.Milliseconds()

					driverData, exists := s.driverGapTrend[s.data[x].Number]
					if !exists {
						s.driverGapTrend[s.data[x].Number] = driverTrend{data: []int64{gap}, trend: 0}
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

						s.driverGapTrend[s.data[x].Number] = driverData
					}
				}
				s.driverGapLock.Unlock()
			}

		case msg := <-s.f.Event():
			s.eventLock.Lock()
			s.event = msg
			s.eventLock.Unlock()

		case msg3 := <-s.f.Time():
			s.eventTime = msg3.Timestamp
			s.remainingTime = msg3.Remaining

		case msg4 := <-s.f.RaceControlMessages():
			s.rcMessagesLock.Lock()
			s.rcMessages = append(s.rcMessages, msg4)
			s.rcMessagesLock.Unlock()

		case msg5 := <-s.f.Radio():
			s.radioLock.Lock()
			s.radio = append(s.radio, msg5)
			s.radioLock.Unlock()

		case msg6 := <-s.f.Weather():
			s.weatherLock.Lock()
			s.weather = msg6
			s.weatherLock.Unlock()
		}
	}

	s.wg.Done()
}

func (s *sessionBase) playTeamRadio() {
	s.wg.Add(1)

	c, ready, err := oto.NewContext(48000, 2, 2)
	if err != nil {
		panic(err)
	}
	<-ready

	for !s.exit.Load() {

		if len(s.radio) > 0 {
			s.radioLock.Lock()
			currentMsg := s.radio[0]
			s.radio = s.radio[1:]
			s.radioLock.Unlock()

			if !s.isMuted {
				if s.play(currentMsg, c) {
					return
				}
			}
		}

		time.Sleep(time.Second * 1)
	}

	s.wg.Done()
}

func (s *sessionBase) play(currentMsg Messages.Radio, c *oto.Context) bool {
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

	s.radioName = currentMsg.Driver

	p := c.NewPlayer(d)
	defer p.Close()
	p.Play()

	for {
		time.Sleep(time.Second)
		if !p.IsPlaying() {
			break
		}
	}

	s.radioName = ""
	return false
}

func (s *sessionBase) webServer() {
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
		w.Write([]byte(s.html))
	})

	for x := range s.servers {

		srv := &http.Server{
			Handler:      router,
			Addr:         s.servers[x],
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		go srv.ListenAndServe()
	}
}

func (s *sessionBase) View() string {
	if s.liveStartTime.After(time.Now()) {
		return lipgloss.Place(s.currentWidth, s.currentHeight, lipgloss.Center, lipgloss.Center,
			fmt.Sprintf("Delaying start, %.1f seconds...", s.liveStartTime.Sub(time.Now()).Seconds()))
	} else if !s.liveDelayExpired {
		s.liveDelayExpired = true
		// Unpause data
		if s.f.IsPaused() {
			s.f.TogglePause()
		}
	}

	hour := int(s.remainingTime.Seconds() / 3600)
	minute := int(s.remainingTime.Seconds()/60) % 60
	second := int(s.remainingTime.Seconds()) % 60
	remaining := fmt.Sprintf("%d:%02d:%02d", hour, minute, second)
	v := make([]Messages.Timing, 0)

	s.dataLock.Lock()
	for _, a := range s.data {
		v = append(v, a)
	}
	s.dataLock.Unlock()

	sort.Slice(v, func(i, j int) bool {
		return v[i].Position < v[j].Position
	})

	segmentCount := s.event.TotalSegments
	if segmentCount == 0 {
		segmentCount = len("Segment")
	}

	// Track the fastest sectors times for the session
	for _, driver := range v {
		if (driver.Sector1 > 0 && driver.Sector1 < s.fastestSector1) || s.fastestSector1 == 0 {
			s.fastestSector1 = driver.Sector1
		}

		if (driver.Sector2 > 0 && driver.Sector2 < s.fastestSector2) || s.fastestSector2 == 0 {
			s.fastestSector2 = driver.Sector2
		}

		if (driver.Sector3 > 0 && driver.Sector3 < s.fastestSector3) || s.fastestSector3 == 0 {
			s.fastestSector3 = driver.Sector3
		}

		if driver.SpeedTrap > s.fastestSpeedTrap {
			s.fastestSpeedTrap = driver.SpeedTrap
		}
	}

	if s.fastestSector1 > 0 && s.fastestSector2 > 0 && s.fastestSector3 > 0 {
		s.theoreticalFastestLap = s.fastestSector1 + s.fastestSector2 + s.fastestSector3
	}

	if s.event.Status == Messages.Started {
		if s.previousSessionActive != Messages.Started {
			s.fastestSector1 = 0
			s.fastestSector2 = 0
			s.fastestSector3 = 0
			s.theoreticalFastestLap = 0
			s.previousSessionActive = s.event.Status
		}
	} else if s.event.Status == Messages.Inactive {
		s.fastestSector1 = 0
		s.fastestSector2 = 0
		s.fastestSector3 = 0
		s.theoreticalFastestLap = 0
		s.previousSessionActive = s.event.Status
	} else {
		s.previousSessionActive = s.event.Status
	}

	table, separator := s.renderDataForScreen(segmentCount, remaining, v)

	table += separator + "\n"
	trackStatus := "Track Status: |"
	for x := 0; x < segmentCount; x++ {
		switch s.event.SegmentFlags[x] {
		case Messages.GreenFlag:
			trackStatus += lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("■")
		case Messages.YellowFlag:
			trackStatus += lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render("■")
		case Messages.DoubleYellowFlag:
			trackStatus += lipgloss.NewStyle().Foreground(lipgloss.Color("#FBFF00")).Render("■")
		case Messages.RedFlag:
			trackStatus += lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("■")
		}
		if x == s.event.Sector1Segments-1 || x == s.event.Sector1Segments+s.event.Sector2Segments-1 {
			trackStatus += "|"
		}
	}
	if s.f.Session() == Messages.RaceSession || s.f.Session() == Messages.SprintSession {
		trackStatus += fmt.Sprintf("|                       |%s|%s|%s|%s|                                    |%s|",
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(s.fastestSector1)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(s.fastestSector2)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(s.fastestSector3)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(s.theoreticalFastestLap)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmt.Sprintf("%d", s.fastestSpeedTrap)))
	} else {
		trackStatus += fmt.Sprintf("|                       |%s|%s|%s|%s|                |%s|",
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(s.fastestSector1)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(s.fastestSector2)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(s.fastestSector3)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmtDuration(s.theoreticalFastestLap)),
			lipgloss.NewStyle().Align(lipgloss.Center).Width(12).Padding(0, 1, 0, 1).Foreground(lipgloss.Color("#D500D5")).Render(fmt.Sprintf("%d", s.fastestSpeedTrap)))
	}

	table += trackStatus + "\n"

	table += separator + "\n"
	s.rcMessagesLock.Lock()
	if len(s.rcMessages) > 0 {
		for x := len(s.rcMessages) - 1; x >= 0 && x >= len(s.rcMessages)-5; x-- {
			lastMessage := s.rcMessages[x]
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

			table += fmt.Sprintf("%s - %s%s\n", lastMessage.Timestamp.In(s.f.CircuitTimezone()).Format("02-01-2006 15:04:05"), prefix, lastMessage.Msg)
		}
	}
	s.rcMessagesLock.Unlock()

	status := separator + "\n"

	s.weatherLock.Lock()
	status += fmt.Sprintf("Air Temp: %.2f°C, Track Temp: %.2f°C, ", s.weather.AirTemp, s.weather.TrackTemp)
	if s.weather.Rainfall {
		status += lipgloss.NewStyle().Foreground(lipgloss.Color("#009DD3")).Render("Raining, ")
	}
	s.weatherLock.Unlock()

	if !s.isMuted {
		status += fmt.Sprintf("Team Radio: On")
	} else {
		status += fmt.Sprintf("Team Radio: Off")
	}

	if s.radioName != "" {
		status += fmt.Sprintf(", Radio: %s", s.radioName)
	}

	// If it is a race and the session hasn't started yet (remaining time count down hasn't started) then
	// display a count down to the start of the session
	if (s.f.Session() == Messages.RaceSession || s.f.Session() == Messages.SprintSession) && s.event.Status == Messages.UnknownState {
		status += ", " + lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(
			fmt.Sprintf("Session Starts in: %s", fmtCountdown(s.f.SessionStart().Sub(s.eventTime))))
	}

	if s.f.IsPaused() {
		status += fmt.Sprintf(", ** PAUSED **")
	}

	table += status

	s.updateHTML(v)

	return table
}

func (s *sessionBase) updateHTML(v []Messages.Timing) {
	hour := int(s.remainingTime.Seconds() / 3600)
	minute := int(s.remainingTime.Seconds()/60) % 60
	second := int(s.remainingTime.Seconds()) % 60
	remaining := fmt.Sprintf("%d:%02d:%02d", hour, minute, second)
	segmentCount := s.event.TotalSegments
	if segmentCount == 0 {
		segmentCount = len("Segment")
	}

	table, separator := s.renderDataForHtml(segmentCount, remaining, v)

	table += separator + "\n"
	trackStatus := "Track Status: |"
	for x := 0; x < segmentCount; x++ {
		switch s.event.SegmentFlags[x] {
		case Messages.GreenFlag:
			trackStatus += "<font color=\"#00FF00\">&#x25a0;</font>"
		case Messages.YellowFlag:
			trackStatus += "<font color=\"#FFFF00\">&#x25a0;</font>"
		case Messages.DoubleYellowFlag:
			trackStatus += "<font color=\"#FBFF00\">&#x25a0;</font>"
		case Messages.RedFlag:
			trackStatus += "<font color=\"#FF0000\">&#x25a0;</font>"
		}
		if x == s.event.Sector1Segments-1 || x == s.event.Sector1Segments+s.event.Sector2Segments-1 {
			trackStatus += "|"
		}
	}
	if s.f.Session() == Messages.RaceSession || s.f.Session() == Messages.SprintSession {
		trackStatus += fmt.Sprintf("|                   |%s|%s|%s|%s|                        |%s|",
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(s.fastestSector1))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(s.fastestSector2))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(s.fastestSector3))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth-2).Render(fmtDuration(s.theoreticalFastestLap))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(fmt.Sprintf("%d", s.fastestSpeedTrap))))
	} else {
		trackStatus += fmt.Sprintf("|                       |%s|%s|%s|%s|                |%s|",
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(s.fastestSector1))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(s.fastestSector2))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(s.fastestSector3))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(timeWidth).Padding(0, 1, 0, 1).Render(fmtDuration(s.theoreticalFastestLap))),
			fmt.Sprintf("<font color=\"#D500D5\">%s</font>", lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Render(fmt.Sprintf("%d", s.fastestSpeedTrap))))
	}

	table += trackStatus + "\n"

	table += separator + "\n"
	s.rcMessagesLock.Lock()
	if len(s.rcMessages) > 0 {
		for x := len(s.rcMessages) - 1; x >= 0 && x >= len(s.rcMessages)-19; x-- {
			lastMessage := s.rcMessages[x]
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

			table += fmt.Sprintf("%s - %s%s\n", lastMessage.Timestamp.In(s.f.CircuitTimezone()).Format("02-01-2006 15:04:05"), prefix, lastMessage.Msg)
		}
	}
	s.rcMessagesLock.Unlock()

	status := separator + "\n"

	s.weatherLock.Lock()
	status += fmt.Sprintf("Air Temp: %.2f°C, Track Temp: %.2f°C", s.weather.AirTemp, s.weather.TrackTemp)
	if s.weather.Rainfall {
		status += ", <font color=\"#009DD3\">Raining</font>"
	}
	s.weatherLock.Unlock()

	// If it is a race and the session hasn't started yet (remaining time count down hasn't started) then
	// display a count down to the start of the session
	if (s.f.Session() == Messages.RaceSession || s.f.Session() == Messages.SprintSession) && s.event.Status == Messages.UnknownState {
		status += fmt.Sprintf(", <font color=\"#00FF00\">Session Starts in: %s</font>", fmtCountdown(s.f.SessionStart().Sub(s.eventTime)))
	}

	table += status

	s.html = table
}
