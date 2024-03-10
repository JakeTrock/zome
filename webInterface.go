package main

import (
	"log"
	"net/http"
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
	// http.HandleFunc("/v1/evt/", a.websocketHandler) //TODO: handle p2p events, channels etc

	log.Fatal(http.ListenAndServe(":5253", nil)) //5j2a5k3e
}

type Request struct {
	Action      string `json:"action"`
	ForceDomain string `json:"forceDomain"`
	Data        []byte `json:"data"`
}
