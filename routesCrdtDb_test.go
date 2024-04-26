package main

import (
	"testing"

	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/stretchr/testify/assert"
)

func TestHandleAddRequest(t *testing.T) {

	randomKeyValuePairs, _, _ := generateRandomStructs()
	addGeneralized(t, randomKeyValuePairs)

}

func TestSetGlobalWrite(t *testing.T) {
	//set global write true on controlSocket
	unmarshalledResponse, err := firstApp.servConn.DbSetGlobalWrite(sharedInterfaces.DbSetGlobalWriteRequest{Value: true})
	assert.NoError(t, err)

	assert.True(t, unmarshalledResponse.DidSucceed)
	// now check if the value was set
	unmarshalledGW, err := firstApp.servConn.DbGetGlobalWrite()
	assert.NoError(t, err)

	assert.True(t, unmarshalledGW.GlobalWrite)

	// now check false
	unmarshalledResponse, err = firstApp.servConn.DbSetGlobalWrite(sharedInterfaces.DbSetGlobalWriteRequest{Value: false})
	assert.NoError(t, err)

	assert.True(t, unmarshalledResponse.DidSucceed)
	// now check if the value was set
	unmarshalledGW, err = firstApp.servConn.DbGetGlobalWrite()
	assert.NoError(t, err)

	assert.True(t, unmarshalledGW.GlobalWrite)
	assert.False(t, unmarshalledGW.GlobalWrite)
}

func TestHandleGetRequest(t *testing.T) {
	randomKeyValuePairs, randomList, _ := generateRandomStructs()

	successBools := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successBools[k] = true
	}

	// Add the key to the store
	addval, err := addGeneralized(t, randomKeyValuePairs)
	assert.NoError(t, err)
	assert.Equal(t, successBools, addval)

	// Get the key from the store
	unmarshalledResponse, err := firstApp.servConn.DbGet(sharedInterfaces.DbGetRequest{
		Values: randomList.values,
	})
	assert.NoError(t, err)
	assert.Equal(t, true, unmarshalledResponse.Success)
	assert.Equal(t, randomKeyValuePairs.values, unmarshalledResponse.Keys)
}

func TestHandleDeleteRequest(t *testing.T) {
	randomKeyValuePairs, randomList, _ := generateRandomStructs()

	successKeys := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successKeys[k] = true
	}

	// Add the key to the store
	addval, err := addGeneralized(t, randomKeyValuePairs)
	assert.NoError(t, err)
	assert.Equal(t, successKeys, addval)

	// Delete the key from the store
	unmarshalledResponse, err := firstApp.servConn.DbDelete(sharedInterfaces.DbDeleteRequest{
		Values: randomList.values,
	})
	assert.NoError(t, err)
	assert.Empty(t, unmarshalledResponse.Error)
	assert.Equal(t, successKeys, unmarshalledResponse.DidSucceed)
}

func addGeneralized(t *testing.T, randomKeyValuePairs keyValueReq) (map[string]bool, error) {
	successKvp := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successKvp[k] = true
	}

	//make add request to controlSocket
	unmarshalledResponse, err := firstApp.servConn.DbAdd(sharedInterfaces.DbAddRequest{
		ACL:    "11",
		Values: randomKeyValuePairs.values,
	})
	assert.NoError(t, err)
	assert.Equal(t, successKvp, unmarshalledResponse.DidSucceed)
	return unmarshalledResponse.DidSucceed, err
}

func TestHandleRemoveOrigin(t *testing.T) {
	//make removeorigin request to controlSocket
	roRes, err := firstApp.servConn.DbRemoveOrigin()
	assert.NoError(t, err)
	assert.True(t, roRes.DidSucceed)
}
