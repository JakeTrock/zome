package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPeers(t *testing.T) {
	// Create a mock App instance
	controlSocket := establishControlSocket()

	type successStruct struct {
		Code   int `json:"code"`
		Status struct {
			Version     string      `json:"version"`
			DbSize      string      `json:"dbsize"`
			DataDirSize string      `json:"dataDirSize"`
			StartTime   string      `json:"startTime"`
			Uptime      string      `json:"uptime"`
			Radar       string      `json:"radar"`
			Peers       []cleanPeer `json:"peers"`
			Error       string      `json:"error"`
		} `json:"status"`
	}

	// now check false
	err := controlSocket.WriteJSON(Request{
		Action: "ps-getPeerStats",
		Data:   struct{}{},
	})
	assert.NoError(t, err)

	unmarshalledResponse := successStruct{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Equal(t, 200, unmarshalledResponse.Code)

	assert.NotEqual(t, "error", unmarshalledResponse.Status.DbSize)
	assert.NotEqual(t, "error", unmarshalledResponse.Status.DataDirSize)
	assert.Equal(t, "jammed", unmarshalledResponse.Status.Radar)
	assert.Empty(t, unmarshalledResponse.Status.Error)
	assert.NotEmpty(t, unmarshalledResponse.Status.Version)
	assert.NotEmpty(t, unmarshalledResponse.Status.StartTime)
	assert.NotEmpty(t, unmarshalledResponse.Status.Uptime)
	// assert.NotEmpty(t, unmarshalledResponse.Status.Peers) //TODO: uncomment when multipeer test is established

}