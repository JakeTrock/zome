package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (a *App) initWeb() {
	http.HandleFunc("/", a.websocketHandler)

	log.Fatal(http.ListenAndServe(":5253", nil)) //5j2a5k3e
}

type Request struct {
	Action      string `json:"action"`
	ForceDomain string `json:"forceDomain"`
	Data        struct {
		ACL    string            `json:"acl"`
		Value  string            `json:"value"`
		Keys   []string          `json:"keys"`
		Values map[string]string `json:"values"`
	} `json:"data"`
}

func (a *App) websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(err)
		return
	}
	defer conn.Close()

	for {
		// Read the message from the client
		_, message, err := conn.ReadMessage()
		if err != nil {
			logger.Error(err)
			return
		}
		origin := r.Header.Get("Origin")
		baseurl, err := url.Parse(origin)
		if err != nil {
			logger.Error(err)
			return
		}
		host := baseurl.Hostname()

		// Decode the message
		var request Request
		err = json.Unmarshal(message, &request)
		if err != nil {
			logger.Error(err)
			return
		}

		println(string(message))

		// Process the request based on the action
		switch request.Action {
		case "add":
			a.handleAddRequest(conn, request, host)
		case "get":
			a.handleGetRequest(conn, request, host)
		case "delete":
			a.handleDeleteRequest(conn, request, host)
		case "setGlobalWrite":
			a.setGlobalWrite(conn, request, host)
		default:
			logger.Error("Invalid action")
			return
		}
	}
}
