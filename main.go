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
	"f1gopher/f1gopher-cmdline/menu"
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/f1gopher/f1gopherlib"
	"log"
	"net"
	"os"
	"time"
)

var Version string
var BuildTime string

func main() {
	cachePtr := flag.String("cache", "./.cache", "Path to the folder to cache data in")
	logPtr := flag.String("log", "", "Log file")
	addressPtr := flag.String("address", "", "Web server address")
	portPtr := flag.String("port", "8000", "Web server port")
	delayPtr := flag.Int("delay", 0, "Live delay in seconds")
	livePtr := flag.Bool("live", false, "Skip menu's and select live feed")
	flag.Parse()

	if len(*logPtr) > 0 {
		f, err := os.OpenFile(*logPtr, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			log.Fatalf("Error creating log file: %v", err)
		}
		defer f.Close()
		f1gopherlib.SetLogOutput(f)
	}

	var servers []string
	if len(*addressPtr) == 0 {
		for _, address := range getLocalIP() {
			servers = append(servers, fmt.Sprintf("%s:%s", address, *portPtr))
		}
	} else {
		servers = []string{fmt.Sprintf("%s:%s", *addressPtr, *portPtr)}
	}

	model := menu.NewUI(*cachePtr, servers, time.Duration(*delayPtr)*time.Second, *livePtr, Version)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithoutCatchPanics())
	p.Run()
}

func getLocalIP() []string {
	ips := []string{"localhost"}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}

	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}
	return ips
}
