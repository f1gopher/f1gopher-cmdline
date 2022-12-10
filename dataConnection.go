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
