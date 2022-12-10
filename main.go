package main

import (
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/f1gopher/f1gopherlib"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	cachePtr := flag.String("cache", "", "Path to the folder to cache data in")
	debugPtr := flag.String("debug", "", "Debug replay file")
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

	model := NewUI(*cachePtr, *debugPtr, servers, time.Duration(*delayPtr)*time.Second, *livePtr)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithoutCatchPanics())
	p.Start()
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
