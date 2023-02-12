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
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/f1gopher/f1gopherlib/Messages"
	"time"
)

type raceUI struct {
	sessionBase
}

func NewRaceUI(servers []string, liveDelay time.Duration) *raceUI {
	ui := &raceUI{
		sessionBase: sessionBase{
			err:       nil,
			data:      make(map[int]Messages.Timing),
			servers:   servers,
			liveDelay: liveDelay,
		},
	}
	ui.renderDataForScreen = ui.uiDisplay
	ui.renderDataForHtml = ui.htmlDisplay

	go ui.webServer()

	return ui
}

func (m *raceUI) uiDisplay(segmentCount int, remaining string, v []Messages.Timing) (table string, separator string) {

	separator = "---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------"

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

	table = title + header + "\n" + separator + "\n"

	for _, driver := range v {

		if driver.Location == Messages.Stopped {
			row := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
				lipgloss.NewStyle().Align(lipgloss.Center).Width(5).Padding(0, 1, 0, 1).Render(fmt.Sprintf("%d", driver.Position)),
				lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(driver.HexColor)).Render(driver.ShortName),
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
		m.driverGapLock.Lock()
		trend, exists := m.driverGapTrend[driver.Number]
		m.driverGapLock.Unlock()
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
			lipgloss.NewStyle().Align(lipgloss.Center).Width(8).Padding(0, 1, 0, 1).Foreground(lipgloss.Color(driver.HexColor)).Render(driver.ShortName),
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

	return table, separator
}

func (m *raceUI) htmlDisplay(segmentCount int, remaining string, v []Messages.Timing) (table string, separator string) {

	separator = "---------------------------------------------------------------------------------------------------------------------------------------------"

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

	table = title + header + "\n" + separator + "\n"

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
		m.driverGapLock.Lock()
		trend, exists := m.driverGapTrend[driver.Number]
		m.driverGapLock.Unlock()
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

	return table, separator
}
