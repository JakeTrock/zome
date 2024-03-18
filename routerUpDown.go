package main

import (
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

//https://gist.github.com/tsilvers/5f827fb11aee027e22c6b3102ebcc497

type UploadHeader struct {
	Filename string
	Size     int64
	Domain   string
}

type DownloadHeader struct {
	Filename     string
	ContinueFrom int64
	Domain       string
}

func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	socket := wsConn{}
	//defer send close frame
	defer socket.killSocket()
	var err error
	//get last string after / in path
	routePath := strings.Split(r.URL.Path, "/")
	uploadId := routePath[len(routePath)-1]
	originSelf := getOriginSegregator(r)
	origin := originSelf

	// Open websocket connection.
	socket.conn, err = upgradeConn(w, r)
	if err != nil {
		logger.Errorf("Error on open of websocket connection: %v", err)
		return
	}
	//check if uploadid in active writes
	header, ok := a.fsActiveWrites[uploadId]
	if !ok {
		socket.sendMessage(400, fmtError("Invalid upload id"))
		return
	}
	if header.Domain != originSelf {
		origin = header.Domain
	}

	pathWithoutFile := path.Join(a.operatingPath, "zome", "data", origin, path.Dir(header.Filename))
	writePath := path.Join(a.operatingPath, "zome", "data", origin, header.Filename)

	// Create data folder if it doesn't exist.
	if _, err = os.Stat(pathWithoutFile); os.IsNotExist(err) {
		if err = os.MkdirAll(pathWithoutFile, 0755); err != nil {
			socket.sendMessage(400, fmtError("Could not create data folder: "+err.Error()))
			return
		}
	}
	// Create temp file to save file.
	var tempFile *os.File
	if tempFile, err = os.Create(writePath); err != nil {
		socket.sendMessage(400, fmtError("Could not create temp file: "+err.Error()))
		return
	}
	defer tempFile.Close()

	// Read file blocks until all bytes are received.
	bytesRead := int64(0)
	for {
		mt, message, err := socket.conn.ReadMessage()
		if err != nil {
			socket.sendMessage(400, fmtError("Error receiving file block: "+err.Error()))
			return
		}
		if mt != websocket.BinaryMessage {
			if mt == websocket.TextMessage {
				if string(message) == "CANCEL" {
					socket.sendMessage(200, ("Upload canceled"))
					//remove uploadid from active writes
					delete(a.fsActiveWrites, uploadId)
					err = os.Remove(tempFile.Name())
					if err != nil {
						socket.sendMessage(500, fmtError("Error removing temp file: "+err.Error()))
					}
					return
				} else if strings.HasPrefix(string(message), "SKIPTO") {
					//if message begins with SKIPTO, skip to that point
					skipIndex, err := strconv.ParseInt(strings.TrimPrefix(string(message), "SKIPTO"), 10, 64)
					socket.sendMessage(200, ("Skipping to " + strconv.FormatInt(skipIndex, 10)))
					if err != nil {
						socket.sendMessage(400, fmtError("Bad skip position: "+err.Error()))
					}
					if skipIndex > header.Size {
						socket.sendMessage(400, fmtError("Bad skip position: greater than file length"))
					}
					bytesRead = skipIndex
					continue
				}

			}
			socket.sendMessage(400, fmtError("Invalid file block received"))
			return
		}

		// tempFile.Write(message)
		_, err = tempFile.WriteAt(message, bytesRead)
		if err != nil {
			socket.sendMessage(500, fmtError("Error writing to temp file: "+err.Error()))
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
		socket.sendMessage(500, fmtError("Error getting file hash: "+err.Error()))
		return
	}

	priorMetaData, error := a.secureGet(header.Filename, origin, originSelf)
	if error != nil {
		socket.sendMessage(500, fmtError("error getting prior metadata "+error.Error()))
		return
	}
	//check if priorMetaData exists, if it does, add the sum key value pair to it
	if priorMetaData != "" {
		//add sum key value pair to priorMetaData
		priorMetaData = strings.TrimSuffix(priorMetaData, "}")
		priorMetaData += ",\"sum\":\"" + sum256 + "\"}"
		_, err = a.secureAdd(header.Filename, priorMetaData, "11", origin, originSelf, true)
		if err != nil {
			socket.sendMessage(500, fmtError("Error adding file to db: "+err.Error()))
			return
		}
	} else {
		socket.sendMessage(500, fmtError("No metadata found for file"))
	}

	successObj := struct {
		DidSucceed bool   `json:"didSucceed"`
		Hash       string `json:"hash"`
		FileName   string `json:"fileName"`
		BytesRead  int64  `json:"bytesRead"`
	}{DidSucceed: true, Hash: sum256, FileName: header.Filename, BytesRead: bytesRead}

	socket.sendMessage(200, successObj)
}

func (a *App) download(w http.ResponseWriter, r *http.Request) {
	socket := wsConn{}
	//defer send close frame
	defer socket.killSocket()
	var err error
	//get last string after / in path
	routePath := strings.Split(r.URL.Path, "/")
	downloadId := routePath[len(routePath)-1]
	originSelf := getOriginSegregator(r)
	origin := originSelf

	socket.conn, err = upgradeConn(w, r)
	if err != nil {
		logger.Error("Error on open of websocket connection:", err)
		return
	}
	//check if uploadid in active writes
	filePath, ok := a.fsActiveReads[downloadId]
	if !ok {
		socket.sendMessage(400, fmtError("Invalid download id"))
		return
	}
	if filePath.Domain != originSelf {
		origin = filePath.Domain
	}

	readPath := path.Join(a.operatingPath, "zome", "data", origin, filePath.Filename)

	fi, err := os.Stat(readPath)
	// Create data folder if it doesn't exist.
	if os.IsNotExist(err) {
		socket.sendMessage(400, fmtError("Could not find data: "+err.Error()))
		return
	}

	fileSize := fi.Size()

	if fileSize == 0 {
		socket.sendMessage(400, fmtError("File is empty"))
		return
	}

	if fileSize < filePath.ContinueFrom {
		socket.sendMessage(400, fmtError("ContinueFrom is greater than file size"))
		return
	}

	fileHandle, err := os.Open(readPath)
	if err != nil {
		socket.sendMessage(500, fmtError("Error opening file: "+err.Error()))
		return
	}

	// send filesize initially
	socket.sendMessage(200, struct {
		Size int64 `json:"size"`
	}{Size: fileSize})

	//consistently write "test" to socket until cancel is received

	// Read file blocks until all bytes are received.
	bytesRead := int64(filePath.ContinueFrom)
	fiBuf := make([]byte, 4096)

	for {
		// shrink fibuf if we are at the end of the file
		if bytesRead+int64(len(fiBuf)) > fileSize {
			fiBuf = make([]byte, fileSize-bytesRead)
		}

		numBytesRead, err := fileHandle.ReadAt(fiBuf, bytesRead)

		if err != nil && err != io.EOF {
			socket.sendMessage(500, fmtError("File error: "+err.Error()))
			return
		}

		bytesRead += int64(numBytesRead)

		if numBytesRead > 0 {
			if err := socket.conn.WriteMessage(websocket.BinaryMessage, fiBuf); err != nil {
				socket.sendMessage(500, fmtError("Socket error: "+err.Error()))
				return
			}
		}

		if err == io.EOF || bytesRead == fileSize { //TODO: timeout if no response from client
			fileHandle.Close()
			break
		}
	}

	// remove uploadid from active writes
	delete(a.fsActiveReads, downloadId)

	successObj := struct {
		DidSucceed bool   `json:"didSucceed"`
		FileName   string `json:"fileName"`
	}{DidSucceed: true, FileName: filePath.Filename}

	socket.sendMessage(200, successObj)
}
