package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleRemoveOrigin(t *testing.T) {
	// Test case 1: Valid request
	expectedSuccess := struct {
		DidSucceed bool `json:"didSucceed"`
	}{
		DidSucceed: true,
	}

	controlSocket := establishControlSocket()

	//make removeorigin request to controlSocket
	err := controlSocket.WriteJSON(Request{
		Action: "db-removeOrigin",
		Data:   []byte(`{}`),
	})
	assert.NoError(t, err)
	//get response
	unmarshalledResponse := struct {
		DidSucceed bool `json:"didSucceed"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)
}

func TestHandleAddRequest(t *testing.T) {
	// Test case 1: Valid request
	randomKeyValuePairs, _, _ := generateRandomStructs()

	requestBody := struct {
		ACL    string            `json:"acl"`
		Values map[string]string `json:"values"`
	}{
		ACL:    "11",
		Values: randomKeyValuePairs.values,
	}

	controlSocket := establishControlSocket()

	requestData, _ := json.Marshal(requestBody)

	successKvp := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successKvp[k] = true
	}

	expectedSuccess := struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}{
		DidSucceed: successKvp,
	}

	//make add request to controlSocket
	err := controlSocket.WriteJSON(Request{
		Action: "db-add",
		Data:   requestData,
	})

	// successJson, err := app.handleAddRequest(nil, request, originKey)
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	// json.Unmarshal(successJson, &unmarshalledResponse)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)
}

func TestSetGlobalWrite(t *testing.T) {
	// Test case 1: Valid request

	controlSocket := establishControlSocket()

	expectedSuccess := struct {
		DidSucceed bool `json:"didSucceed"`
	}{
		DidSucceed: true,
	}

	setGlobalWriteTrue := Request{
		Data: []byte(`{"value":"true"}`),
	}
	setGlobalWriteFalse := Request{
		Data: []byte(`{"value":"false"}`),
	}

	//set global write true on controlSocket
	err := controlSocket.WriteJSON(Request{
		Action: "db-setGlobalWrite",
		Data:   setGlobalWriteTrue.Data,
	})
	assert.NoError(t, err)

	unmarshalledResponse := struct {
		DidSucceed bool `json:"didSucceed"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)

	assert.Equal(t, expectedSuccess, unmarshalledResponse)
	// now check if the value was set
	err = controlSocket.WriteJSON(Request{
		Action: "db-getGlobalWrite",
		Data:   []byte(`{}`),
	})
	assert.NoError(t, err)
	unmarshalledGW := struct {
		GlobalWrite bool `json:"globalWrite"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledGW)
	assert.NoError(t, err)

	assert.True(t, unmarshalledGW.GlobalWrite)

	// now check false
	err = controlSocket.WriteJSON(Request{
		Action: "db-setGlobalWrite",
		Data:   setGlobalWriteFalse.Data,
	})
	assert.NoError(t, err)

	unmarshalledResponse = struct {
		DidSucceed bool `json:"didSucceed"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)
	// now check if the value was set
	err = controlSocket.WriteJSON(Request{
		Action: "db-getGlobalWrite",
		Data:   []byte(`{}`),
	})
	assert.NoError(t, err)
	unmarshalledGW = struct {
		GlobalWrite bool `json:"globalWrite"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledGW)
	assert.NoError(t, err)
	assert.False(t, unmarshalledGW.GlobalWrite)
}

func TestHandleGetRequest(t *testing.T) {

	// Test case 1: Valid request
	randomKeyValuePairs, randomList, _ := generateRandomStructs()

	controlSocket := establishControlSocket()

	requestBody := struct {
		Values []string `json:"values"`
	}{
		Values: randomList.values,
	}
	requestData, _ := json.Marshal(requestBody)
	getRequest := Request{
		Data: requestData,
	}

	expectedSuccess := struct {
		DidSucceed bool              `json:"didSucceed"`
		Values     map[string]string `json:"keys"`
	}{
		DidSucceed: true,
		Values:     randomKeyValuePairs.values,
	}

	successBools := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successBools[k] = true
	}

	// Add the key to the store
	addval, err := addGeneralized(randomKeyValuePairs, originBase)
	assert.NoError(t, err)
	assert.Equal(t, successBools, addval)

	// Get the key from the store
	err = controlSocket.WriteJSON(Request{
		Action: "db-get",
		Data:   getRequest.Data,
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		DidSucceed bool              `json:"didSucceed"`
		Values     map[string]string `json:"keys"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)
}

func TestHandleDeleteRequest(t *testing.T) {
	// Test case 1: Valid request
	randomKeyValuePairs, randomList, _ := generateRandomStructs()

	controlSocket := establishControlSocket()

	requestBody := struct {
		Values []string `json:"values"`
	}{
		Values: randomList.values,
	}
	requestData, _ := json.Marshal(requestBody)
	request := Request{
		Data: requestData,
	}

	successKeys := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successKeys[k] = true
	}

	expectedSuccess := struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}{
		DidSucceed: successKeys,
	}

	// Add the key to the store
	addval, err := addGeneralized(randomKeyValuePairs, originBase)
	assert.NoError(t, err)
	assert.Equal(t, expectedSuccess.DidSucceed, addval)

	// Delete the key from the store
	unmarshalledResponse := struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}{}
	err = controlSocket.WriteJSON(Request{
		Action: "db-delete",
		Data:   request.Data,
	})
	assert.NoError(t, err)
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)
}
