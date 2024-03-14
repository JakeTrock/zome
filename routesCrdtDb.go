package main

import (
	"encoding/json"

	ds "github.com/ipfs/go-datastore"
)

func (a *App) removeOrigin(wc wsConn, _ []byte, selfOrigin string) {
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

func (a *App) setGlobalWrite(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Request
		Data struct {
			Value bool `json:"value"`
		} `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
	}

	if requestBody.Data.Value != true && requestBody.Data.Value != false {
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

	enableByte := []byte{0}
	if requestBody.Data.Value {
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
}

func (a *App) handleAddRequest(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Action      string `json:"action"`
		ForceDomain string `json:"forceDomain"`
		Data        struct {
			ACL    string            `json:"acl"`
			Values map[string]string `json:"values"`
		} `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
	}

	//validate request
	if len(requestBody.Data.Values) == 0 {
		wc.sendMessage(400, ("invalid request body: no values provided"))
		return
	}

	if requestBody.Data.ACL == "" {
		requestBody.Data.ACL = "11"
	}

	type successReturn struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}

	var origin = selfOrigin
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}

	successResult, err := a.secureAddLoop(requestBody.Data.Values, requestBody.Data.ACL, origin, selfOrigin)
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

func (a *App) handleGetRequest(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Action      string `json:"action"`
		ForceDomain string `json:"forceDomain"`
		Data        struct {
			Values []string `json:"values"`
		} `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
	}

	if len(requestBody.Data.Values) == 0 {
		wc.sendMessage(400, ("invalid request body: no keys provided"))
		return
	}

	type returnMessage struct {
		Success bool              `json:"didSucceed"`
		Keys    map[string]string `json:"keys"`
	}

	var origin = selfOrigin
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}

	getResult, err := a.secureGetLoop(requestBody.Data.Values, origin, selfOrigin)
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

func (a *App) handleDeleteRequest(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Action      string `json:"action"`
		ForceDomain string `json:"forceDomain"`
		Data        struct {
			Values []string `json:"values"`
		} `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
	}

	if len(requestBody.Data.Values) == 0 {
		wc.sendMessage(400, ("invalid request body: no keys provided"))
		return
	}

	type successReturn struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: make(map[string]bool, len(requestBody.Data.Values)),
	}

	for _, k := range requestBody.Data.Values {
		origin := selfOrigin + "-" + k
		if requestBody.ForceDomain != "" {
			origin = requestBody.ForceDomain + "-" + k
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

func (a *App) getGlobalWrite(wc wsConn, _ []byte, selfOrigin string) {
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
