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
	"github.com/f1gopher/f1gopherlib"
	"github.com/f1gopher/f1gopherlib/flowControl"
	"github.com/f1gopher/f1gopherlib/parser"
)

func newLiveConnection(cache string) f1gopherlib.F1GopherLib {
	data, _ := f1gopherlib.CreateLive(
		parser.EventTime|parser.Timing|parser.Event|parser.RaceControl|parser.TeamRadio|parser.Weather,
		"",
		cache)

	return data
}

func newReplayConnection(cache string, event f1gopherlib.RaceEvent) f1gopherlib.F1GopherLib {
	data, _ := f1gopherlib.CreateReplay(
		parser.EventTime|parser.Timing|parser.Event|parser.RaceControl|parser.TeamRadio|parser.Weather,
		event,
		cache,
		flowControl.Realtime)

	return data
}
