package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gorilla/websocket"
)

const HandshakeTimeoutSecs = 10

var upgrader = websocket.Upgrader{
	HandshakeTimeout: time.Second * HandshakeTimeoutSecs,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (a *App) initWeb() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./frontend/index.html")
	})

	http.HandleFunc("/v1/control/", a.websocketCRDTHandler)
	http.HandleFunc("/v1/upload/", a.upload)
	http.HandleFunc("/v1/download/", a.download)
	// http.HandleFunc("/v1/evt/", a.websocketHandler) //TODO: handle p2p events, channels etc

	log.Fatal(http.ListenAndServe(":5253", nil)) //5j2a5k3e
}

type Request struct {
	Action      string `json:"action"`
	ForceDomain string `json:"forceDomain"`
	Data        []byte `json:"data"`
}

func killSocket(conn *websocket.Conn) {
	if r := recover(); r != nil {
		fmt.Printf("Recovered from crash: %v", r)
		debug.PrintStack()
	}
	if conn == nil {
		return
	}
	err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		logger.Error(err)
	}
	fmt.Println("Closing socket", time.Now())
	conn.Close()
}
