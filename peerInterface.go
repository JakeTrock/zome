package main

import (
	"encoding/json"
	"runtime/debug"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// key you use to get your approved peers
const approvedConnKey = "friendServers"

func (a *App) checkApprovedPeer(socket network.Stream, streamPipe func(peerConn)) {
	originString := string(socket.Conn().RemotePeer())

	keyBin, err := a.secureInternalKeyGet(approvedConnKey)
	if err != nil {
		a.Logger.Error(err)
		return
	}
	var approvedConns struct {
		Keys string `json:"keys"`
	}
	err = json.Unmarshal(keyBin, &approvedConns)
	if err != nil {
		a.Logger.Error(err)
		return
	}

	// check that this client is allowed connection to you
	pc := peerConn{
		conn:   socket,
		origin: originString,
		Logger: a.Logger,
	}
	if strings.Contains(approvedConns.Keys, originString) {
		go streamPipe(pc)
	} else {
		// send them a nice connection denial message
		pc.sendMessage(403, "{\"error\":\"ACCESS DENIED\"}")
		pc.killSocket()
	}
}

func (a *App) initInterface() {
	// listen for new connections on ctrSocket protocol
	ctrSocket := protocol.ID("/zomeCtrSocket/1.0.0")
	a.host.SetStreamHandler(ctrSocket, func(socket network.Stream) {
		a.checkApprovedPeer(socket, a.routeControl)
	})
	dlSocket := protocol.ID("/zomeDlSocket/1.0.0")
	a.host.SetStreamHandler(dlSocket, func(socket network.Stream) {
		a.checkApprovedPeer(socket, a.routeDownload)
	})
	ulSocket := protocol.ID("/zomeUlSocket/1.0.0")
	a.host.SetStreamHandler(ulSocket, func(socket network.Stream) {
		a.checkApprovedPeer(socket, a.routeUpload)
	})
}

type peerConn struct {
	conn   network.Stream
	origin string
	Logger *logging.ZapEventLogger
}

func (pc peerConn) sendMessage(code int, status any) {
	if code == 500 {
		pc.Logger.Error(status)
	}
	res := sharedInterfaces.ResBody{Code: code}
	res.Status = status
	msg, err := json.Marshal(res)
	if err != nil {
		pc.Logger.Error(err)
		return
	}
	_, err = pc.conn.Write(msg)
	if err != nil {
		pc.Logger.Error(err)
		return
	}
}

func (pc peerConn) ReadMessage() (int, []byte, error) {
	// read until message is received
	message := []byte{}
	_, err := pc.conn.Read(message)
	if err != nil {
		return 0, nil, err
	}
	return 0, message, nil
}

//BEGIN FNS for download signalling

func (pc peerConn) writeBinary(data []byte) error {
	// add b character to data
	data = append([]byte("b"), data...)
	_, err := pc.conn.Write(data)
	return err
}

func (pc peerConn) writeRawStr(data string) error {
	// add s character to data
	data = "s" + data
	_, err := pc.conn.Write([]byte(data))
	return err
}

type messageTypes int

const (
	binaryMsg messageTypes = iota //binary
	stringMsg                     //string
	errMsg                        //error
)

func (pc peerConn) readFileSocket() (messageTypes, []byte, error) {
	// read until message is received
	message := []byte{}
	_, err := pc.conn.Read(message)
	if err != nil {
		return errMsg, nil, err
	}
	// remove initial character
	initialChar := string(message[0])
	mType := errMsg
	switch initialChar {
	case "b":
		mType = binaryMsg
	case "s":
		mType = stringMsg
	default:
		pc.Logger.Error("Invalid message type")
		return errMsg, nil, nil
	}

	message = message[1:]
	return mType, message, nil
}

//END FNS for download signalling

func (pc peerConn) killSocket() {
	if r := recover(); r != nil {
		pc.Logger.Infof("Recovered from crash: %v", r)
		debug.PrintStack()
	}
	if pc.conn == nil {
		return
	}
	err := pc.conn.Close()
	if err != nil {
		pc.Logger.Error(err)
	}
	pc.Logger.Info("Closing socket", time.Now())
	pc.conn.Close()
}

func fmtError(err string) sharedInterfaces.EResp {
	return sharedInterfaces.EResp{
		DidSucceed: false,
		Error:      err,
	}
}
