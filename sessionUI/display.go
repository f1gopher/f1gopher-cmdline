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

func fmtDuration(d time.Duration) string {
	milliseconds := d.Milliseconds()

	if milliseconds == 0 {
		return ""
	}

	minutes := milliseconds / (1000 * 60)
	milliseconds -= minutes * 60 * 1000
	seconds := milliseconds / 1000
	milliseconds -= seconds * 1000

	isNegative := false
	if minutes < 0 {
		minutes = -minutes
		isNegative = true
	}
	if seconds < 0 {
		seconds = -seconds
		isNegative = true
	}
	if milliseconds < 0 {
		milliseconds = -milliseconds
		isNegative = true
	}

	if isNegative {
		return fmt.Sprintf("-%02d:%02d.%03d", minutes, seconds, milliseconds)
	}

	// If no minutes then don't display zero but pad with spaces for display alignment
	if minutes == 0 {
		return fmt.Sprintf("   %02d.%03d", seconds, milliseconds)
	}

	return fmt.Sprintf("%02d:%02d.%03d", minutes, seconds, milliseconds)
}

func fmtCountdown(d time.Duration) string {
	milliseconds := d.Milliseconds()

	if milliseconds == 0 {
		return ""
	}

	minutes := milliseconds / (1000 * 60)
	milliseconds -= minutes * 60 * 1000
	seconds := milliseconds / 1000
	milliseconds -= seconds * 1000

	if minutes < 0 {
		minutes = -minutes
	}
	if seconds < 0 {
		seconds = -seconds
	}

	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func timeColor(personalFastest bool, overallFastest bool) string {

	if overallFastest {
		return "#D500D5"
	} else if personalFastest {
		return "#00FF00"
	} else {
		return "#FFFF00"
	}
}

func segmentColor(segmentType Messages.SegmentType) lipgloss.Color {
	switch segmentType {
	case Messages.YellowSegment:
		return "#FFFF00"
	case Messages.GreenSegment:
		return "#00FF00"
	case Messages.InvalidSegment:
		return "#000000"
	case Messages.PurpleSegment:
		return "#D500D5"
	case Messages.RedSegment:
		return "#FF0000"
	case Messages.PitlaneSegment:
		return "#FFFFFF"
	case Messages.Mystery, Messages.Mystery2, Messages.Mystery3:
		return "#0000FF"
	default:
		return "#FFFFFF"
	}
}

func fastestLapColor(overallFastest bool) string {

	if overallFastest {
		return "#D500D5"
	} else {
		// TODO - use default text color
		return "#B4B0B0"
	}
}

func tireColor(tire Messages.TireType) string {
	switch tire {
	case Messages.Soft:
		return "#FF0000"
	case Messages.Medium:
		return "#FFFF00"
	case Messages.Hard:
		return "#FFFFFF"
	case Messages.Intermediate:
		return "#00D300"
	case Messages.Wet:
		return "#00A9FF"
	default:
		return "#6917AE"
	}
}

func trackStatusColor(state Messages.FlagState) string {
	switch state {
	case Messages.GreenFlag:
		return "#00FF00"
	case Messages.YellowFlag, Messages.DoubleYellowFlag:
		return "#FFFF00"
	case Messages.RedFlag:
		return "#FF0000"
	case Messages.ChequeredFlag:
		return "#FFFFFF"
	case Messages.NoFlag:
		return ""
	default:
		panic("Unhandled track status color: " + state.String())
	}
}

func sessionStatusColor(state Messages.SessionState) string {
	switch state {
	case Messages.UnknownState, Messages.Inactive, Messages.Finished, Messages.Finalised, Messages.Ended:
		return "#FFFFFF"
	case Messages.Started:
		return "#00FF00"
	case Messages.Aborted:
		return "#FF0000"
	default:
		panic("Unhandled session status color: " + state.String())
	}
}

func safetyCarFormat(state Messages.TrackState) string {

	var color = "#00FF00"

	switch state {
	case Messages.VirtualSafetyCar, Messages.VirtualSafetyCarEnding:
		color = "#FFFF00"
	case Messages.SafetyCar, Messages.SafetyCarEnding:
		color = "#FF0000"
	}

	return color
}

func locationColor(location Messages.CarLocation) string {
	switch location {
	case Messages.Pitlane, Messages.PitOut, Messages.NoLocation:
		return "#FFFFFF"
	case Messages.OnTrack, Messages.OutLap:
		return "#00FF00"
	case Messages.Stopped, Messages.OutOfRace:
		return "#FF0000"
	default:
		panic("Unhandled location color: " + location.String())
	}
}
