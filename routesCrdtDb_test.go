package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleRemoveOrigin(t *testing.T) {
	// Test case 1: Valid request

	controlSocket := establishControlSocket()

	//make removeorigin request to controlSocket
	err := controlSocket.WriteJSON(Request{
		Action: "db-removeOrigin",
	})
	assert.NoError(t, err)
	//get response
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Equal(t, 200, unmarshalledResponse.Code)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)
}

func TestHandleAddRequest(t *testing.T) {
	// Test case 1: Valid request
	randomKeyValuePairs, _, _ := generateRandomStructs()
	addGeneralized(t, randomKeyValuePairs)

}

func TestSetGlobalWrite(t *testing.T) {
	// Test case 1: Valid request

	controlSocket := establishControlSocket()

	type successStruct struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			Error      string `json:"error"`
		} `json:"status"`
	}

	type gwStruct struct {
		Code   int `json:"code"`
		Status struct {
			GlobalWrite bool   `json:"globalWrite"`
			Error       string `json:"error"`
		} `json:"status"`
	}

	//set global write true on controlSocket
	err := controlSocket.WriteJSON(Request{
		Action: "db-setGlobalWrite",
		Data: struct {
			Value bool `json:"value"`
		}{Value: true},
	})
	assert.NoError(t, err)

	unmarshalledResponse := successStruct{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)

	assert.True(t, unmarshalledResponse.Status.DidSucceed)
	// now check if the value was set
	err = controlSocket.WriteJSON(Request{
		Action: "db-getGlobalWrite",
	})
	assert.NoError(t, err)
	unmarshalledGW := gwStruct{}
	err = controlSocket.ReadJSON(&unmarshalledGW)
	assert.NoError(t, err)

	assert.True(t, unmarshalledGW.Status.GlobalWrite)

	// now check false
	err = controlSocket.WriteJSON(Request{
		Action: "db-setGlobalWrite",
		Data: struct {
			Value bool `json:"value"`
		}{Value: false},
	})
	assert.NoError(t, err)

	unmarshalledResponse = successStruct{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)
	// now check if the value was set
	err = controlSocket.WriteJSON(Request{
		Action: "db-getGlobalWrite",
	})
	assert.NoError(t, err)
	unmarshalledGW = gwStruct{}
	err = controlSocket.ReadJSON(&unmarshalledGW)
	assert.NoError(t, err)
	assert.False(t, unmarshalledGW.Status.GlobalWrite)
}

func TestHandleGetRequest(t *testing.T) {

	// Test case 1: Valid request
	randomKeyValuePairs, randomList, _ := generateRandomStructs()

	controlSocket := establishControlSocket()

	type getStruct struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool              `json:"didSucceed"`
			Values     map[string]string `json:"keys"`
		} `json:"status"`
	}

	getRequest := Request{
		Data: struct {
			Values []string `json:"values"`
		}{
			Values: randomList.values,
		},
	}

	expectedSuccess := getStruct{
		Code: 200,
		Status: struct {
			DidSucceed bool              `json:"didSucceed"`
			Values     map[string]string `json:"keys"`
		}{
			DidSucceed: true,
			Values:     randomKeyValuePairs.values,
		},
	}

	successBools := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successBools[k] = true
	}

	// Add the key to the store
	addval, err := addGeneralized(t, randomKeyValuePairs)
	assert.NoError(t, err)
	assert.Equal(t, successBools, addval)

	// Get the key from the store
	err = controlSocket.WriteJSON(Request{
		Action: "db-get",
		Data:   getRequest.Data,
	})
	assert.NoError(t, err)
	unmarshalledResponse := getStruct{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)
}

func TestHandleDeleteRequest(t *testing.T) {
	// Test case 1: Valid request
	randomKeyValuePairs, randomList, _ := generateRandomStructs()

	controlSocket := establishControlSocket()

	request := Request{
		Data: struct {
			Values []string `json:"values"`
		}{
			Values: randomList.values,
		},
	}

	type deleteStruct struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed map[string]bool `json:"didSucceed"`
			Error      string          `json:"error"`
		} `json:"status"`
	}

	successKeys := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successKeys[k] = true
	}

	// Add the key to the store
	addval, err := addGeneralized(t, randomKeyValuePairs)
	assert.NoError(t, err)
	assert.Equal(t, successKeys, addval)

	// Delete the key from the store
	unmarshalledResponse := deleteStruct{}
	err = controlSocket.WriteJSON(Request{
		Action: "db-delete",
		Data:   request.Data,
	})
	assert.NoError(t, err)
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Empty(t, unmarshalledResponse.Status.Error)
	assert.Equal(t, successKeys, unmarshalledResponse.Status.DidSucceed)
	assert.Equal(t, 200, unmarshalledResponse.Code)
}

func addGeneralized(t *testing.T, randomKeyValuePairs keyValueReq) (map[string]bool, error) {
	requestBody := struct {
		ACL    string            `json:"acl"`
		Values map[string]string `json:"values"`
	}{
		ACL:    "11",
		Values: randomKeyValuePairs.values,
	}

	controlSocket := establishControlSocket()

	successKvp := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successKvp[k] = true
	}

	type successStruct struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed map[string]bool `json:"didSucceed"`
			Error      string          `json:"error"`
		} `json:"status"`
	}

	//make add request to controlSocket
	err := controlSocket.WriteJSON(Request{
		Action: "db-add",
		Data:   requestBody,
	})
	// successJson, err := app.handleAddRequest(nil, request, originKey)
	assert.NoError(t, err)
	unmarshalledResponse := successStruct{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.Equal(t, successKvp, unmarshalledResponse.Status.DidSucceed)
	assert.Equal(t, 200, unmarshalledResponse.Code)
	return unmarshalledResponse.Status.DidSucceed, err
}
