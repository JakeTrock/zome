package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
)

func (a *App) websocketCRDTHandler(w http.ResponseWriter, r *http.Request) { //TODO: where will these requests come from?
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

		host := getOriginSegregator(r)

		// Decode the message
		var request Request
		err = json.Unmarshal(message, &request)
		if err != nil {
			logger.Error(err)
			return
		}

		println(string(message))

		// Create the success response
		success := []byte{}
		// Process the request based on the action
		// DB routes
		switch request.Action {
		case "db-add":
			success, err = a.handleAddRequest(conn, request, host)
		case "db-get":
			success, err = a.handleGetRequest(conn, request, host)
		case "db-delete":
			success, err = a.handleDeleteRequest(conn, request, host)
		case "db-setGlobalWrite":
			success, err = a.setGlobalWrite(conn, request, host)
		case "db-getGlobalWrite":
			success, err = a.getGlobalWrite(conn, request, host)
		case "db-removeOrigin":
			success, err = a.removeOrigin(conn, request, host)

		// FS routes
		case "fs-putObject":
			success, err = a.PutObjectRoute(conn, request, host)

		// ADmin routes
		case "ad-getServerStats":
			success, err = a.getServerStats(conn, request, host)
		//TODO: admin routes for blocking origins, fs restrictions
		default:
			logger.Error("Invalid action")
			return
		}

		if err != nil {
			errObject := struct {
				Error string `json:"error"`
			}{
				Error: err.Error(),
			}
			errByte, err := json.Marshal(errObject)
			if err != nil {
				logger.Error(err)
				return
			}
			err = conn.WriteMessage(websocket.TextMessage, errByte)
			if err != nil {
				logger.Error(err)
				return
			}
		} else {
			// Send the success response to the client
			err = conn.WriteMessage(websocket.TextMessage, success)
			if err != nil {
				logger.Error(err)
				return
			}
		}
	}
}

func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func (a *App) getServerStats(conn *websocket.Conn, _ Request, _ string) ([]byte, error) { //TODO: consider security of this route
	type returnMessage struct {
		Stats map[string]string `json:"stats"`
	}

	returnMessages := returnMessage{
		Stats: make(map[string]string, 1),
	}

	returnMessages.Stats["version"] = "1.0.0"
	dbSize, err := ds.DiskUsage(a.ctx, a.store)
	if err != nil {
		logger.Error(err)
		returnMessages.Stats["dbSize"] = "error"
	} else {
		returnMessages.Stats["dbSize"] = fmt.Sprint(dbSize)
	}

	dataDirSize, err := DirSize(a.operatingPath) //TODO: this route can be used for DoS
	if err != nil {
		logger.Error(err)
		returnMessages.Stats["dataDirSize"] = "error"
	} else {
		returnMessages.Stats["dataDirSize"] = fmt.Sprint(dataDirSize)
	}

	returnMessages.Stats["startTime"] = a.startTime.String()
	returnMessages.Stats["uptime"] = fmt.Sprint(time.Since(a.startTime).String())

	returnMessages.Stats["radar"] = "jammed" //I've lost the bleeps, I've lost the sweeps, and I've lost the creeps

	// Encode the return messages
	retMessages, err := json.Marshal(returnMessages)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	// Send the return messages to the client
	err = conn.WriteMessage(websocket.TextMessage, retMessages)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	return retMessages, nil
}

func getOriginSegregator(r *http.Request) string {
	origin := r.Header.Get("Origin") //TODO: is there a better way to segregate requests, this has no bearing on client apps
	baseurl, err := url.Parse(origin)
	if err != nil {
		panic(fmt.Errorf("error parsing origin: %s", err))
	}
	host := baseurl.Hostname()
	if baseurl.Port() != "" {
		host = host + ":" + baseurl.Port()
	}
	return host
}
