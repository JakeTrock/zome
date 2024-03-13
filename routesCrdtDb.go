package main

import (
	"encoding/json"

	ds "github.com/ipfs/go-datastore"
)

func (a *App) removeOrigin(wc wsConn, _ Request, selfOrigin string) {
	type successReturn struct {
		DidSucceed bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: false,
	}
	err := a.store.DB.DropPrefix([]byte(selfOrigin))
	if err != nil {
		wc.sendMessage(500, (err.Error()))
		success.DidSucceed = false
	} else {
		success.DidSucceed = true
	}

	wc.sendMessage(200, success)
	return
}

func (a *App) setGlobalWrite(wc wsConn, request Request, selfOrigin string) {
	var requestBody struct {
		Value string `json:"value"`
	}
	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {

		wc.sendMessage(400, (err.Error()))
		return
	}

	if requestBody.Value != "true" && requestBody.Value != "false" {
		wc.sendMessage(400, ("invalid request body: value must be true or false"))
		return
	}

	type successReturn struct {
		DidSucceed bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: false,
	}

	origin := selfOrigin + "]-GW"

	enableBool := requestBody.Value == "true"
	enableByte := []byte{0}
	if enableBool {
		enableByte = []byte{1}
	}

	err = a.store.Put(a.ctx, ds.NewKey(origin), enableByte)
	if err != nil {
		success.DidSucceed = false
		wc.sendMessage(500, (err.Error()))
	} else {
		success.DidSucceed = true
	}

	wc.sendMessage(200, success)
	return
}

func (a *App) handleAddRequest(wc wsConn, request Request, selfOrigin string) { //TODO: switch to using badger write
	var requestBody struct {
		ACL    string            `json:"acl"`
		Values map[string]string `json:"values"`
	}
	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
		return
	}

	//validate request
	if len(requestBody.Values) == 0 {
		wc.sendMessage(400, ("invalid request body: no values provided"))
		return
	}

	if requestBody.ACL == "" {
		requestBody.ACL = "11"
	}

	type successReturn struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}

	var origin = selfOrigin
	if request.ForceDomain != "" {
		origin = request.ForceDomain
	}

	successResult, err := a.secureAddLoop(requestBody.Values, requestBody.ACL, origin, selfOrigin)
	if err != nil {

		wc.sendMessage(500, (err.Error()))
		return
	}

	success := successReturn{
		DidSucceed: successResult,
	}

	wc.sendMessage(200, success)
	return
}

func (a *App) handleGetRequest(wc wsConn, request Request, selfOrigin string) {
	var requestBody struct {
		Values []string `json:"values"`
	}
	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
		return
	}

	if len(requestBody.Values) == 0 {
		wc.sendMessage(400, ("invalid request body: no keys provided"))
		return
	}

	type returnMessage struct {
		Success bool              `json:"didSucceed"`
		Keys    map[string]string `json:"keys"`
	}

	var origin = selfOrigin
	if request.ForceDomain != "" {
		origin = request.ForceDomain
	}

	getResult, err := a.secureGetLoop(requestBody.Values, origin, selfOrigin)
	if err != nil {
		wc.sendMessage(500, (err.Error()))
		return
	}

	returnMessages := returnMessage{
		Success: false,
		Keys:    getResult,
	}

	returnMessages.Success = true

	wc.sendMessage(200, returnMessages)
	return
}

func (a *App) handleDeleteRequest(wc wsConn, request Request, selfOrigin string) {
	var requestBody struct {
		Values []string `json:"values"`
	}
	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
		return
	}

	if len(requestBody.Values) == 0 {
		wc.sendMessage(400, ("invalid request body: no keys provided"))
		return
	}

	type successReturn struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: make(map[string]bool, len(requestBody.Values)),
	}

	for _, k := range requestBody.Values {
		origin := selfOrigin + "-" + k
		if request.ForceDomain != "" {
			origin = request.ForceDomain + "-" + k
		}
		// Retrieve the value from the store
		value, err := a.store.Get(a.ctx, ds.NewKey(origin))
		if err != nil {
			success.DidSucceed[k] = false

			continue
		}
		//check ACL
		if checkACL(string(value[:2]), "3", selfOrigin, origin) {
			success.DidSucceed[k] = false
			continue
		}
		// Delete the key from the store
		err = a.store.Delete(a.ctx, ds.NewKey(origin))
		if err != nil {
			success.DidSucceed[k] = false

		} else {
			success.DidSucceed[k] = true
		}
	}

	wc.sendMessage(200, success)
	return
}

func (a *App) getGlobalWrite(wc wsConn, _ Request, selfOrigin string) {
	gwrite, err := a.globalWriteAbstract(selfOrigin)
	if err != nil {
		wc.sendMessage(500, (err.Error()))
		return
	}
	successObj := struct {
		GlobalWrite bool `json:"globalWrite"`
	}{}
	successObj.GlobalWrite = gwrite

	wc.sendMessage(200, successObj)
	return
}
