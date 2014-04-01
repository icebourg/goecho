package main

import (
	"fmt"
	"github.com/cactus/go-statsd-client/statsd"
	"github.com/tatsushid/go-fastping"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type response struct {
	addr *net.IPAddr
	rtt  time.Duration
	fail bool
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s {statsdhost}\n", os.Args[0])
		os.Exit(1)
	}

	// go channels that we work responses off of
	onRecv, onIdle := make(chan *response), make(chan bool)

	// construct our pinger
	pinger := fastping.NewPinger()

	// get targets file of IPs
	targetcfg, err := ioutil.ReadFile("targets.cfg")
	if err != nil {
		fmt.Println("Unable to read targets.cfg")
		return
	}

	targets := strings.Split(string(targetcfg), "\n")

	hostname, err := os.Hostname()
	if err != nil {
		fmt.Println("Mommy why didn't you give me a name?")
		os.Exit(1)
	}

	// initiate statsd stuff
	statsd, err := statsd.Dial(os.Args[1], fmt.Sprintf("goecho.%s", hostname))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
	defer statsd.Close()

	// get all the IPs and add them to our pinger
	for _, target := range targets {
		ip, err := net.ResolveIPAddr("ip4:icmp", target)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		pinger.AddIPAddr(ip)

		pinger.AddHandler("receive", func(addr *net.IPAddr, t time.Duration) {
			onRecv <- &response{addr: addr, rtt: t}
		})

		pinger.AddHandler("idle", func() {
			onIdle <- true
		})
	}

	quit := pinger.RunLoop()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)

loop:
	for {
		select {
		case <-c:
			fmt.Println("my dearest genevieve, i shall live another day")
			break loop
		case res := <-onRecv:
			ms := int64(res.rtt / time.Millisecond)
			key := strings.Replace(fmt.Sprintf("%s", res.addr), ".", "_", -1)

			fmt.Printf("%s \t \t %v\n", res.addr, res.rtt)
			statsd.Timing(key, ms, 1)
		case <-onIdle:
			fmt.Printf("idle")
		}
	}

	wait := make(chan bool)
	quit <- wait
	<-wait
}
