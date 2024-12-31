package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	//Global variables
	stopchan chan bool

	//Runtime flags
	benchmarkWebserverListeningPort int
)

func init() {
	flag.IntVar(&benchmarkWebserverListeningPort, "port", 8123, "Port to listen on")
	flag.Parse()
}

/* SIGTERM handler, do shutdown sequences before closing */
func SetupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		//Stop all request loops
		fmt.Println("Stopping request generators")
		if stopchan != nil {
			stopchan <- true
		}

		// Wait for all goroutines to finish
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
}

func main() {
	//Setup the SIGTERM handler
	SetupCloseHandler()
	//Start the web server
	fmt.Println("Starting web server on port", benchmarkWebserverListeningPort)
	fmt.Println("In Zoraxy, point your test proxy rule to this server at the given port")
	startWebServer()
	select {}
}
