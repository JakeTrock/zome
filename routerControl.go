package main

import (
	"encoding/json"
)

func (a *App) routeControl(socket peerConn) {

	for {
		// Read the message from the client
		_, message, err := socket.ReadMessage()
		if err != nil {
			a.Logger.Error(err)
			return
		}

		host := socket.origin
		//peer id the message is from

		// Decode the message
		var request struct {
			Action string `json:"action"`
		}
		err = json.Unmarshal(message, &request)
		if err != nil {
			a.Logger.Error(err)
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
			a.getDirectoryListing(socket, message, host)

		default:
			// a.Logger.Error("Invalid action")
			socket.sendMessage(400, "{\"error\":\"Invalid action\"}")
			return
		}
	}
}
