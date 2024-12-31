package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func echo(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	for key, values := range r.Header {
		for _, value := range values {
			message := fmt.Sprintf("%s: %s", key, value)
			if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
				log.Println("WriteMessage error:", err)
				return
			}
		}
	}

	if err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
		log.Println("CloseMessage error:", err)
		return
	}
}

func main() {
	http.HandleFunc("/echo", echo)
	log.Fatal(http.ListenAndServe(":8888", nil))
}
