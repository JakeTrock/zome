package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

//https://gist.github.com/tsilvers/5f827fb11aee027e22c6b3102ebcc497

type UploadHeader struct {
	Filename string
	Size     int64
}

type UploadStatus struct {
	Code   int    `json:"code,omitempty"`
	Status string `json:"status,omitempty"`
	Pct    *int   `json:"pct,omitempty"` // File processing AFTER upload is done.
	pct    int
}

type wsConn struct {
	conn *websocket.Conn
}

func (wsc wsConn) sendStatus(code int, status string) {
	if msg, err := json.Marshal(UploadStatus{Code: code, Status: status}); err == nil {
		wsc.conn.WriteMessage(websocket.TextMessage, msg)
		if code == 500 {
			fmt.Println("ECODE: ", err)
		}
	} //TODO: if status code is 500 range log it, use a logger
}

func (wsc wsConn) sendPct(pct int) {
	stat := UploadStatus{pct: pct}
	stat.Pct = &stat.pct
	if msg, err := json.Marshal(stat); err == nil {
		wsc.conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	socket := wsConn{}
	//defer send close frame
	defer killSocket(socket.conn)
	var err error
	//get last string after / in path
	routePath := strings.Split(r.URL.Path, "/")
	uploadId := routePath[len(routePath)-1]
	origin := getOriginSegregator(r)
	//TODO: how to do cross origin uploads?

	// Open websocket connection.
	upgrader := websocket.Upgrader{
		HandshakeTimeout: time.Second * HandshakeTimeoutSecs,
		CheckOrigin: func(r *http.Request) bool {
			ogHeader := r.Header.Get("Origin")
			//check ogheader is a valid url
			return ogHeader != ""
		},
	}
	socket.conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error on open of websocket connection:", err)
		return
	}
	//check if uploadid in active writes
	header, ok := a.fsActiveWrites[uploadId]
	if !ok {
		socket.sendStatus(400, "Invalid upload id")
		return
	}

	pathWithoutFile := path.Join(a.operatingPath, "zome", "data", origin, path.Dir(header.Filename))
	writePath := path.Join(a.operatingPath, "zome", "data", origin, header.Filename)

	// Create data folder if it doesn't exist.
	if _, err = os.Stat(pathWithoutFile); os.IsNotExist(err) {
		if err = os.MkdirAll(pathWithoutFile, 0755); err != nil {
			socket.sendStatus(400, "Could not create data folder: "+err.Error())
			return
		}
	}
	// Create temp file to save file.
	var tempFile *os.File
	if tempFile, err = os.Create(writePath); err != nil {
		socket.sendStatus(400, "Could not create temp file: "+err.Error())
		return
	}
	defer tempFile.Close()

	// Read file blocks until all bytes are received.
	bytesRead := int64(0)
	for {
		mt, message, err := socket.conn.ReadMessage()
		if err != nil {
			socket.sendStatus(400, "Error receiving file block: "+err.Error())
			return
		}
		if mt != websocket.BinaryMessage {
			if mt == websocket.TextMessage {
				if string(message) == "CANCEL" {
					socket.sendStatus(200, "Upload canceled")
					//remove uploadid from active writes
					delete(a.fsActiveWrites, uploadId)
					err = os.Remove(tempFile.Name())
					if err != nil {
						socket.sendStatus(500, "Error removing temp file: "+err.Error())
					}
					return
				} else if strings.HasPrefix(string(message), "SKIPTO") {
					//if message begins with SKIPTO, skip to that point
					skipIndex, err := strconv.ParseInt(strings.TrimPrefix(string(message), "SKIPTO"), 10, 64)
					socket.sendStatus(200, fmt.Sprintf("Skipping to %d", skipIndex))
					if err != nil {
						socket.sendStatus(400, "Bad skip position: "+err.Error())
					}
					if skipIndex > header.Size {
						socket.sendStatus(400, "Bad skip position: greater than file length")
					}
					bytesRead = skipIndex
					continue
				}

			}
			socket.sendStatus(400, "Invalid file block received")
			return
		}

		// tempFile.Write(message)
		_, err = tempFile.WriteAt(message, bytesRead)
		if err != nil {
			socket.sendStatus(500, "Error writing to temp file: "+err.Error())
			return
		}
		bytesRead += int64(len(message))
		if bytesRead >= header.Size {
			tempFile.Close()
			break
		}

		socket.sendPct(int((bytesRead * 100) / header.Size))
	}

	// remove uploadid from active writes
	delete(a.fsActiveWrites, uploadId)
	// calculate file sha256
	sum256, err := sha256File(writePath)
	if err != nil {
		socket.sendStatus(400, "Error getting file hash: "+err.Error())
		return
	}
	writeObj := map[string]string{}
	writeObj[header.Filename] = sum256

	_, err = a.secureAddLoop(writeObj, "11", origin, origin) //TODO: handle cross origin uploads
	if err != nil {
		socket.sendStatus(400, "Error adding file to db: "+err.Error())
		return
	}

	successObj := struct {
		DidSucceed bool   `json:"didSucceed"`
		Hash       string `json:"hash"`
		FileName   string `json:"fileName"`
		BytesRead  int64  `json:"bytesRead"`
	}{DidSucceed: true, Hash: sum256, FileName: header.Filename, BytesRead: bytesRead}

	// Encode the success response
	successJson, err := json.Marshal(successObj)
	if err != nil {
		socket.sendStatus(400, "Error encoding success response: "+err.Error())
		return
	}
	socket.sendStatus(200, string(successJson))
}

func (a *App) download(w http.ResponseWriter, r *http.Request) {
	socket := wsConn{}
	//defer send close frame
	defer killSocket(socket.conn)
	var err error
	//get last string after / in path
	routePath := strings.Split(r.URL.Path, "/")
	downloadId := routePath[len(routePath)-1]
	origin := getOriginSegregator(r)
	//TODO: how to do cross origin downloads?

	// Open websocket connection.
	upgrader := websocket.Upgrader{
		HandshakeTimeout: time.Second * HandshakeTimeoutSecs,
		CheckOrigin: func(r *http.Request) bool {
			ogHeader := r.Header.Get("Origin")
			//check ogheader is a valid url
			return ogHeader != ""
		},
	}
	socket.conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error on open of websocket connection:", err)
		return
	}
	//check if uploadid in active writes
	filePath, ok := a.fsActiveReads[downloadId]
	if !ok {
		socket.sendStatus(400, "Invalid download id")
		return
	}

	readPath := path.Join(a.operatingPath, "zome", "data", origin, filePath)

	fi, err := os.Stat(readPath)
	// Create data folder if it doesn't exist.
	if os.IsNotExist(err) {
		socket.sendStatus(400, "Could not find data: "+err.Error())
		return
	}

	fileSize := fi.Size()

	fileHandle, err := os.Open(readPath)
	if err != nil {
		socket.sendStatus(500, "Error opening file: "+err.Error())
		return
	}

	// send filesize initially
	socket.sendStatus(200, "{\"size\":"+strconv.FormatInt(fileSize, 10)+"}")

	//consistently write "test" to socket until cancel is received

	// Read file blocks until all bytes are received.
	bytesRead := int64(0)
	fiBuf := make([]byte, 4096) //TODO: allow for changes in the buf size, or oh no... CONGESTION CONTROL ACK ACK ACK(geddit? :P)
	for {
		// mt, message, err := socket.conn.ReadMessage()//TODO: this is blocking, implement a different channel/use existing ctrl?
		// if err != nil {
		// 	socket.sendStatus(400, "Error receiving file ACK: "+err.Error())
		// 	return
		// }
		// if mt == websocket.TextMessage {
		// 	if string(message) == "CANCEL" {
		// 		socket.sendStatus(200, "Download canceled")
		// 		//remove downloadid from active reads
		// 		delete(a.fsActiveReads, downloadId)
		// 		return
		// 	} else if strings.HasPrefix(string(message), "SKIPTO") {
		// 		//if message begins with SKIPTO, skip to that point
		// 		skipIndex, err := strconv.ParseInt(strings.TrimPrefix(string(message), "SKIPTO"), 10, 64)
		// 		if err != nil {
		// 			socket.sendStatus(400, "Bad skip position: "+err.Error())
		// 		}
		// 		if skipIndex > fileSize {
		// 			socket.sendStatus(400, "Bad skip position: greater than file length")
		// 		}
		// 		bytesRead = skipIndex
		// 	}
		// } else {
		// 	socket.sendStatus(400, "Invalid ctrl block received")
		// 	return
		// }

		// shrink fibuf if we are at the end of the file
		if bytesRead+int64(len(fiBuf)) > fileSize {
			fiBuf = make([]byte, fileSize-bytesRead)
		}

		numBytesRead, err := fileHandle.ReadAt(fiBuf, bytesRead)

		if err != nil && err != io.EOF {
			socket.sendStatus(500, "File error: "+err.Error())
			return
		}

		bytesRead += int64(numBytesRead)

		if numBytesRead > 0 {
			if err := socket.conn.WriteMessage(websocket.BinaryMessage, fiBuf); err != nil {
				socket.sendStatus(500, "Socket error: "+err.Error())
				return
			}
		}

		if err == io.EOF || bytesRead == fileSize {
			fileHandle.Close()
			break
		}
	}

	// remove uploadid from active writes
	delete(a.fsActiveReads, downloadId)

	successObj := struct {
		DidSucceed bool   `json:"didSucceed"`
		FileName   string `json:"fileName"`
	}{DidSucceed: true, FileName: filePath}

	// Encode the success response
	successJson, err := json.Marshal(successObj)
	if err != nil {
		socket.sendStatus(400, "Error encoding success response: "+err.Error())
		return
	}
	socket.sendStatus(200, string(successJson))
}
