package main

import (
	"encoding/json"
	"fmt"
	"time"

	ds "github.com/ipfs/go-datastore"
)

//TODO: testme

func (a *App) getPeerStats(wc wsConn, request []byte, selfOrigin string) {
	var returnMessage = struct {
		Version     string      `json:"version"`
		DbSize      string      `json:"dbsize"`
		DataDirSize string      `json:"dataDirSize"`
		StartTime   string      `json:"startTime"`
		Uptime      string      `json:"uptime"`
		Radar       string      `json:"radar"`
		Peers       []cleanPeer `json:"peers"`
	}{
		Version:     zomeVersion,
		DbSize:      "error",
		DataDirSize: "error",
		StartTime:   a.startTime.String(),
		Uptime:      fmt.Sprint(time.Since(a.startTime).String()),
		Peers:       connectedPeersClean(a.host),
		Radar:       "jammed", //I've lost the bleeps, I've lost the sweeps, and I've lost the creeps
	}

	dbSize, err := ds.DiskUsage(a.ctx, a.store)
	if err != nil {
		logger.Error(err)
		wc.sendMessage(500, "error getting db size"+err.Error())
		return
	} else {
		returnMessage.DbSize = fmt.Sprint(dbSize)
	}

	dataDirSize, err := DirSize(a.operatingPath) //TODO: this route can be used for DoS
	if err != nil {
		logger.Error(err)
		wc.sendMessage(500, "error getting data dir size"+err.Error())
		return
	} else {
		returnMessage.DataDirSize = fmt.Sprint(dataDirSize)
	}

	// Send the return messages to the client
	wc.sendMessage(200, returnMessage)
}

//TODO: establishTunnel(high speed 1:1 connection between two peers(tcp/udp?) sendMessage(blank sends to all)), moar!

func (a *App) sendMessage(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Action      string `json:"action"`
		ForceDomain string `json:"forceDomain"`
		Data        struct {
			PeerIDs []string `json:"peerID"`
			Data    string   `json:"data"`
		} `json:"data"`
	}
	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		logger.Error(err)
		wc.sendMessage(500, "error unmarshalling message"+err.Error())
		return
	}

	// //get first key of a.topics //TODO: don't publish to all topics, this should be reimplemented
	err = a.topic.Publish(a.ctx, []byte(requestBody.Data.Data))
	if err != nil {
		logger.Error(err)
		wc.sendMessage(500, "error publishing message"+err.Error())
		return
	}

}
