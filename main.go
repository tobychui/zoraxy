package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"imuslab.com/arozos/ReverseProxy/mod/aroz"
	"imuslab.com/arozos/ReverseProxy/mod/database"
)

var (
	handler *aroz.ArozHandler
	sysdb   *database.Database
)

//Kill signal handler. Do something before the system the core terminate.
func SetupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("\r- Shutting down demo module.")
		//Do other things like close database or opened files
		sysdb.Close()

		os.Exit(0)
	}()
}

func main() {
	//Start the aoModule pipeline (which will parse the flags as well). Pass in the module launch information
	handler = aroz.HandleFlagParse(aroz.ServiceInfo{
		Name:        "ReverseProxy",
		Desc:        "Basic reverse proxy listener",
		Group:       "Network",
		IconPath:    "reverseproxy/img/small_icon.png",
		Version:     "0.1",
		StartDir:    "reverseproxy/index.html",
		SupportFW:   true,
		LaunchFWDir: "reverseproxy/index.html",
		SupportEmb:  false,
		InitFWSize:  []int{1080, 580},
	})

	//Register the standard web services urls
	fs := http.FileServer(http.Dir("./web"))

	http.Handle("/", fs)

	SetupCloseHandler()

	//Create database
	db, err := database.NewDatabase("sys.db", false)
	if err != nil {
		log.Fatal(err)
	}

	sysdb = db

	//Start the reverse proxy server in go routine
	go func() {
		ReverseProxtInit()
	}()

	//Any log println will be shown in the core system via STDOUT redirection. But not STDIN.
	log.Println("ReverseProxy started. Listening on " + handler.Port)
	err = http.ListenAndServe(handler.Port, nil)
	if err != nil {
		log.Fatal(err)
	}

}
