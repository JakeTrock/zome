package main

import (
	"encoding/json"
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
		ogHeader := r.Header.Get("Origin")
		//check ogheader is a valid url
		return ogHeader != ""
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
	Data        any    `json:"data"`
}

type ResBody struct {
	Code   int `json:"code,omitempty"`
	Status any `json:"status,omitempty"`
	Error  any `json:"error,omitempty"`
}

// type JSONBody struct {
// 	Code   int         `json:"code,omitempty"`
// 	Status interface{} `json:"status,omitempty"`
// }

type wsConn struct {
	conn *websocket.Conn
}

func (wsc wsConn) sendMessage(code int, status any) {
	fmt.Println(status) //TODO: remove
	if code == 500 {
		fmt.Println("ECODE: ", status)
		logger.Error(status)
	}
	res := ResBody{Code: code}
	if code != 200 {
		res.Error = status
	} else {
		res.Status = status
	}
	msg, err := json.Marshal(res)
	if err != nil {
		logger.Error(err)
		return
	}
	err = wsc.conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		logger.Error(err)
		return
	}
}

type UploadStatus struct {
	Code   int    `json:"code,omitempty"`
	Status string `json:"status,omitempty"`
	Pct    *int   `json:"pct,omitempty"` // File processing AFTER upload is done.
	pct    int
}

func (wsc wsConn) sendPct(pct int) {
	stat := UploadStatus{pct: pct}
	stat.Pct = &stat.pct
	if msg, err := json.Marshal(stat); err == nil {
		wsc.conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func (wsc wsConn) killSocket() {
	if r := recover(); r != nil {
		fmt.Printf("Recovered from crash: %v", r)
		debug.PrintStack()
	}
	if wsc.conn == nil {
		return
	}
	err := wsc.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		logger.Error(err)
	}
	fmt.Println("Closing socket", time.Now())
	wsc.conn.Close()
}

func upgradeConn(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	return conn, nil
}
