package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
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

		// Decode the message
		var request struct {
			Action string `json:"action"`
			Data   struct {
				Keys   []string          `json:"keys"`
				Values map[string]string `json:"values"`
			} `json:"data"`
		}
		err = json.Unmarshal(message, &request)
		if err != nil {
			logger.Error(err)
			return
		}

		// Process the request based on the action
		switch request.Action {
		case "add":
			a.handleAddRequest(conn, request.Data.Values)
		case "get":
			a.handleGetRequest(conn, request.Data.Keys)
		case "delete":
			a.handleDeleteRequest(conn, request.Data.Keys)
		default:
			logger.Error("Invalid action")
			return
		}
	}
}

func (a *App) handleAddRequest(conn *websocket.Conn, keys map[string]string) {
	type successReturn struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: make(map[string]bool),
	}

	for k, v := range keys {
		// Add the key-value pair to the store
		err := a.store.Put(a.ctx, ds.NewKey(k), []byte(v))
		if err != nil {
			success.DidSucceed[k] = false
			logger.Error(err)
		} else {
			success.DidSucceed[k] = true
		}
	}

	// Encode the success response
	successJson, err := json.Marshal(success)
	if err != nil {
		logger.Error(err)
		return
	}

	// Send the success response to the client
	err = conn.WriteMessage(websocket.TextMessage, successJson)
	if err != nil {
		logger.Error(err)
		return
	}
}

func (a *App) handleGetRequest(conn *websocket.Conn, keys []string) {
	type returnMessage struct {
		Keys map[string]string `json:"keys"`
	}

	returnMessages := returnMessage{
		Keys: make(map[string]string),
	}

	for _, k := range keys {
		// Retrieve the value from the store
		value, err := a.store.Get(a.ctx, ds.NewKey(k))
		if err != nil {
			logger.Error(err)
		} else {
			returnMessages.Keys[k] = string(value)
		}
	}

	// Encode the return messages
	retMessages, err := json.Marshal(returnMessages)
	if err != nil {
		logger.Error(err)
		return
	}

	// Send the return messages to the client
	err = conn.WriteMessage(websocket.TextMessage, retMessages)
	if err != nil {
		logger.Error(err)
		return
	}
}

func (a *App) handleDeleteRequest(conn *websocket.Conn, keys []string) {
	type successReturn struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: make(map[string]bool),
	}

	for _, k := range keys {
		// Delete the key from the store
		err := a.store.Delete(a.ctx, ds.NewKey(k))
		if err != nil {
			success.DidSucceed[k] = false
			logger.Error(err)
		} else {
			success.DidSucceed[k] = true
		}
	}

	// Encode the success response
	successJson, err := json.Marshal(success)
	if err != nil {
		logger.Error(err)
		return
	}

	// Send the success response to the client
	err = conn.WriteMessage(websocket.TextMessage, successJson)
	if err != nil {
		logger.Error(err)
		return
	}
}
