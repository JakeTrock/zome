package main

import (
	"encoding/json"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

var app = &App{}

const testPath = "./testPath"

// setup tests
func setupSuite() {
	if app.startTime.IsZero() {
		app.Startup(map[string]string{"configPath": testPath})
		app.InitP2P()
	}
}

func TestMain(m *testing.M) {
	//clear the testPath
	os.RemoveAll(testPath)
	setupSuite()
	code := m.Run()
	// Perform any teardown or cleanup here
	app.Shutdown()
	os.Exit(code)
}

func TestHandleRemoveOrigin(t *testing.T) {
	// Test case 1: Valid request
	request := Request{}
	originKey := "originKey1"

	expectedSuccess := struct {
		DidSucceed bool `json:"didSucceed"`
	}{
		DidSucceed: true,
	}

	successJson, err := app.removeOrigin(nil, request, originKey)
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		DidSucceed bool `json:"didSucceed"`
	}{}
	json.Unmarshal(successJson, &unmarshalledResponse)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)

	// Add more test cases as needed
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
	requestData, _ := json.Marshal(requestBody)
	request := Request{
		Data: requestData,
	}
	originKey := "originKey1"

	successKvp := map[string]bool{}
	for k := range randomKeyValuePairs.values {
		successKvp[k] = true
	}

	expectedSuccess := struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}{
		DidSucceed: successKvp,
	}

	successJson, err := app.handleAddRequest(nil, request, originKey)
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}{}
	json.Unmarshal(successJson, &unmarshalledResponse)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)

	// Test case 2: Invalid request body
	requestBody.Values = map[string]string{}
	requestData, _ = json.Marshal(requestBody)
	request = Request{
		Data: requestData,
	}

	_, err = app.handleAddRequest(nil, request, originKey)
	assert.Error(t, err)
	assert.EqualError(t, err, "invalid request body: no values provided")

	// Add more test cases as needed
}

func TestSetGlobalWrite(t *testing.T) {
	// Test case 1: Valid request
	origin := "origin1"

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

	successJson, err := app.setGlobalWrite(nil, setGlobalWriteTrue, origin)
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		DidSucceed bool `json:"didSucceed"`
	}{}
	json.Unmarshal(successJson, &unmarshalledResponse)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)
	// now check if the value was set
	getGlobalWrite, err := app.getGlobalWrite(origin)
	assert.NoError(t, err)
	assert.True(t, getGlobalWrite)

	// now check false
	successJson, err = app.setGlobalWrite(nil, setGlobalWriteFalse, origin)
	assert.NoError(t, err)
	json.Unmarshal(successJson, &unmarshalledResponse)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)
	// now check if the value was set
	getGlobalWrite, err = app.getGlobalWrite(origin)
	assert.NoError(t, err)
	assert.False(t, getGlobalWrite)

	// Add more test cases as needed
}

func TestHandleGetRequest(t *testing.T) {

	// Test case 1: Valid request
	randomKeyValuePairs, randomList, _ := generateRandomStructs()

	requestBody := struct {
		Values []string `json:"values"`
	}{
		Values: randomList.values,
	}
	requestData, _ := json.Marshal(requestBody)
	getRequest := Request{
		Data: requestData,
	}
	originKey := "originKey1"

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
	addval, err := addGeneralized(randomKeyValuePairs, originKey)
	assert.NoError(t, err)
	assert.Equal(t, successBools, addval)

	// Get the key from the store
	successJson, err := app.handleGetRequest(nil, getRequest, originKey)
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		DidSucceed bool              `json:"didSucceed"`
		Values     map[string]string `json:"keys"`
	}{}
	json.Unmarshal(successJson, &unmarshalledResponse)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)

	// Test case 2: Invalid request
	getRequest = Request{
		Data: []byte(`{"key":""}`),
	}

	_, err = app.handleGetRequest(nil, getRequest, originKey)
	assert.Error(t, err)
	assert.EqualError(t, err, "invalid request body: no keys provided")

	// Add more test cases as needed
}

func TestHandleDeleteRequest(t *testing.T) {
	// Test case 1: Valid request
	randomKeyValuePairs, randomList, _ := generateRandomStructs()

	requestBody := struct {
		Values []string `json:"values"`
	}{
		Values: randomList.values,
	}
	requestData, _ := json.Marshal(requestBody)
	request := Request{
		Data: requestData,
	}
	originKey := "originKey1"

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
	addval, err := addGeneralized(randomKeyValuePairs, originKey)
	assert.NoError(t, err)
	assert.Equal(t, expectedSuccess.DidSucceed, addval)

	// Delete the key from the store
	unmarshalledResponse := struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}{}
	successJson, err := app.handleDeleteRequest(nil, request, originKey)
	assert.NoError(t, err)
	json.Unmarshal(successJson, &unmarshalledResponse)
	assert.Equal(t, expectedSuccess, unmarshalledResponse)

	// Test case 2: Invalid request
	request = Request{
		Data: []byte(`{"key":""}`),
	}

	_, err = app.handleDeleteRequest(nil, request, originKey)
	assert.Error(t, err)
	assert.EqualError(t, err, "invalid request body: no keys provided")

	// Add more test cases as needed
}

func generateRandomKey() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 5)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

type keyValueReq struct {
	values map[string]string
}

type listReq struct {
	values []string
}

type singleReq struct {
	value string
}

func generateRandomStructs() (keyValueReq, listReq, singleReq) {
	single := generateRandomKey()
	randomKeyValuePairs := keyValueReq{
		values: map[string]string{
			generateRandomKey(): generateRandomKey(),
			generateRandomKey(): generateRandomKey(),
		},
	}

	randomKeyValuePairs.values[single] = generateRandomKey()

	randomList := listReq{
		values: []string{},
	}

	for k := range randomKeyValuePairs.values {
		randomList.values = append(randomList.values, k)
	}

	randomSingle := singleReq{
		value: single,
	}

	return randomKeyValuePairs, randomList, randomSingle
}

func addGeneralized(randomKeyValuePairs keyValueReq, originKey string) (map[string]bool, error) {
	addRequestBody := struct {
		ACL    string            `json:"acl"`
		Values map[string]string `json:"values"`
	}{
		ACL:    "11",
		Values: randomKeyValuePairs.values,
	}
	addRequestData, _ := json.Marshal(addRequestBody)
	addRequest := Request{
		Data: addRequestData,
	}
	// Add the key to the store
	successJson, err := app.handleAddRequest(nil, addRequest, originKey)
	if err != nil {
		return nil, err
	}
	unmarshalledResponse := struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}{}
	json.Unmarshal(successJson, &unmarshalledResponse)
	return unmarshalledResponse.DidSucceed, nil
}
