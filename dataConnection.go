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

package main

import (
	"github.com/f1gopher/f1gopherlib"
	"github.com/f1gopher/f1gopherlib/Messages"
	"github.com/f1gopher/f1gopherlib/parser"
	"time"
)

func NewLiveConnection(cache string) f1gopherlib.F1GopherLib {
	_, data := f1gopherlib.CreateLive(
		parser.EventTime|parser.Timing|parser.Event|parser.RaceControl|parser.TeamRadio|parser.Weather,
		"./archive/abu dhabi/race",
		cache)

	return data
}

func NewReplayConnection(cache string, event f1gopherlib.RaceEvent) f1gopherlib.F1GopherLib {
	data, _ := f1gopherlib.CreateReplay(
		parser.EventTime|parser.Timing|parser.Event|parser.RaceControl|parser.TeamRadio|parser.Weather,
		event,
		cache)

	return data
}

func NewDebugReplayConnection(cache string, dataFile string) f1gopherlib.F1GopherLib {

	event := f1gopherlib.CreateRaceEvent(
		"United States",
		time.Date(2022, 10, 23, 0, 0, 0, 0, time.UTC),
		time.Date(2022, 10, 22, 0, 0, 0, 0, time.UTC),
		Messages.QualifyingSession,
		"",
		time.UTC.String(),
		"",
		"")

	data, _ := f1gopherlib.CreateLiveReplay(
		parser.EventTime|parser.Timing|parser.Event|parser.RaceControl|parser.TeamRadio|parser.Weather,
		dataFile, *event,
		cache)

	return data
}
