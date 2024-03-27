package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func (a *App) websocketCRDTHandler(w http.ResponseWriter, r *http.Request) { //TODO: where will these requests come from?
	socket := wsConn{}
	//defer send close frame
	defer socket.killSocket()
	var err error
	socket.conn, err = upgradeConn(w, r)

	if err != nil {
		logger.Error(err)
		return
	}

	for {
		// Read the message from the client
		_, message, err := socket.conn.ReadMessage()
		if err != nil {
			logger.Error(err)
			return
		}

		host := getOriginSegregator(r)

		// Decode the message
		var request struct {
			Action string `json:"action"`
		}
		err = json.Unmarshal(message, &request)
		if err != nil {
			logger.Error(err)
			return
		}

		// DB routes
		switch request.Action {
		case "db-add":
			a.handleAddRequest(socket, message, host)
		case "db-get":
			a.handleGetRequest(socket, message, host)
		case "db-delete":
			a.handleDeleteRequest(socket, message, host)
		case "db-setGlobalWrite":
			a.setGlobalWrite(socket, message, host)
		case "db-getGlobalWrite":
			a.getGlobalWrite(socket, message, host)
		case "db-removeOrigin":
			a.removeOrigin(socket, message, host)

		// FS routes
		case "fs-put":
			a.putObjectRoute(socket, message, host)
		case "fs-get":
			a.getObjectRoute(socket, message, host)
		case "fs-delete":
			a.deleteObjectRoute(socket, message, host)
		case "fs-getGlobalWrite":
			a.getGlobalFACL(socket, message, host)
		case "fs-setGlobalWrite":
			a.setGlobalFACL(socket, message, host)
		case "fs-removeOrigin":
			a.removeObjectOrigin(socket, message, host)
		case "fs-getListing":
			a.getDirectoryListing(socket, message, host) //TODO: all of these routes over p2p, only admin over ws

		// ADmin routes
		case "ad-getPeerStats":
			a.getPeerStats(socket, message, host)

		//TODO: admin routes for blocking origins, fs restrictions
		default:
			// logger.Error("Invalid action")
			socket.sendMessage(400, "{\"error\":\"Invalid action\"}")
			return
		}
	}
}

func getOriginSegregator(r *http.Request) string {
	origin := r.Header.Get("Origin") //TODO: replace this with just getting the pubkey from the p2p request
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
