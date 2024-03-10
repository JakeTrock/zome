package main

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
)

func (a *App) removeOrigin(_ *websocket.Conn, _ Request, selfOrigin string) ([]byte, error) {
	type successReturn struct {
		DidSucceed bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: false,
	}
	err := a.store.DB.DropPrefix([]byte(selfOrigin))
	if err != nil {
		logger.Error(err)
		success.DidSucceed = false
	} else {
		success.DidSucceed = true
	}

	successJson, err := json.Marshal(success)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	return successJson, nil
}

func (a *App) setGlobalWrite(_ *websocket.Conn, request Request, selfOrigin string) ([]byte, error) {
	var requestBody struct {
		Value string `json:"value"`
	}
	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	if requestBody.Value != "true" && requestBody.Value != "false" {
		logger.Error("invalid request body: value must be true or false")
		return nil, fmt.Errorf("invalid request body: value must be true or false")
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
		logger.Error(err)
	} else {
		success.DidSucceed = true
	}

	// Encode the success response
	successJson, err := json.Marshal(success)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	return successJson, nil
}

func (a *App) handleAddRequest(_ *websocket.Conn, request Request, selfOrigin string) ([]byte, error) { //TODO: switch to using badger write
	var requestBody struct {
		ACL    string            `json:"acl"`
		Values map[string]string `json:"values"`
	}
	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	//validate request
	if len(requestBody.Values) == 0 {
		logger.Error("Invalid request body: no values provided")
		return nil, fmt.Errorf("invalid request body: no values provided")
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
		logger.Error(err)
		return nil, err
	}

	success := successReturn{
		DidSucceed: successResult,
	}

	// Encode the success response
	successJson, err := json.Marshal(success)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	return successJson, nil
}

func (a *App) handleGetRequest(_ *websocket.Conn, request Request, selfOrigin string) ([]byte, error) {
	var requestBody struct {
		Values []string `json:"values"`
	}
	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	if len(requestBody.Values) == 0 {
		logger.Error("Invalid request body: no keys provided")
		return nil, fmt.Errorf("invalid request body: no keys provided")
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
		logger.Error(err)
		return nil, err
	}

	returnMessages := returnMessage{
		Success: false,
		Keys:    getResult,
	}

	// Encode the return messages
	returnMessages.Success = true
	retMessages, err := json.Marshal(returnMessages)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	return retMessages, nil
}

func (a *App) handleDeleteRequest(_ *websocket.Conn, request Request, selfOrigin string) ([]byte, error) {
	var requestBody struct {
		Values []string `json:"values"`
	}
	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	if len(requestBody.Values) == 0 {
		logger.Error("Invalid request body: no keys provided")
		return nil, fmt.Errorf("invalid request body: no keys provided")
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
			logger.Error(err)
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
			logger.Error(err)
		} else {
			success.DidSucceed[k] = true
		}
	}

	// Encode the success response
	successJson, err := json.Marshal(success)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	return successJson, nil
}

func (a *App) getGlobalWrite(_ *websocket.Conn, _ Request, selfOrigin string) ([]byte, error) {
	gwrite, err := a.globalWriteAbstract(selfOrigin)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	successObj := struct {
		GlobalWrite bool `json:"globalWrite"`
	}{}
	successObj.GlobalWrite = gwrite
	success, err := json.Marshal(successObj)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	return success, nil
}

// utils

func (a *App) globalWriteAbstract(origin string) (bool, error) {
	selfOrigin := origin + "]-GW"
	value, err := a.store.Get(a.ctx, ds.NewKey(selfOrigin))
	if err != nil {
		if err != ds.ErrNotFound {
			return false, err
		} else {
			//repair db to secure state
			err = a.store.Put(a.ctx, ds.NewKey(selfOrigin+"]-GW"), []byte{0})
			if err != nil {
				return false, err
			}
			return false, nil
		}
	}
	if value[0] == 1 {
		return true, nil
	}
	return false, nil
}

func (a *App) secureAddLoop(addValues map[string]string, ACL string, origin string, selfOrigin string) (map[string]bool, error) {

	success := make(map[string]bool, len(addValues))

	acl, err := sanitizeACL(ACL)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	var globalWrite = false
	if origin != selfOrigin {
		globalWrite, err = a.globalWriteAbstract(origin)
		if err != nil {
			logger.Error(err)
			return nil, err
		}
	} else {
		globalWrite = true
	}

	for k, v := range addValues {
		origin := origin + "-" + k
		// check value exists
		didSucceed, err := a.secureAdd(origin, v, acl, origin, globalWrite)
		if err != nil {
			logger.Error(err)
			success[k] = false
		}
		success[k] = didSucceed
	}
	return success, nil
}

func (a *App) secureGetLoop(getValues []string, origin string, selfOrigin string) (map[string]string, error) {
	success := make(map[string]string, len(getValues))

	for _, k := range getValues {
		origin := origin + "-" + k
		// Retrieve the value from the store
		decryptedValue, err := a.secureGet(origin, selfOrigin)
		if err != nil {
			logger.Error(err)
			success[k] = ""
		}
		success[k] = decryptedValue
	}
	return success, nil
}

func (a *App) secureAdd(origin string, valueText string, acl string, selfOrigin string, globalWrite bool) (bool, error) {
	priorValue, err := a.store.Get(a.ctx, ds.NewKey(origin))
	if err != nil {
		if err != ds.ErrNotFound {
			return false, err
		} else if !globalWrite { // only continue if the global write is enabled
			return false, nil
		}
	} else if checkACL(string(priorValue[:2]), "2", selfOrigin, origin) {
		return false, nil
	}

	var encBytes []byte
	// Encrypt the value with the private key
	encryptedValue, err := AesGCMEncrypt(a.dbCryptKey, []byte(valueText))
	if err != nil {
		logger.Error(err)
		return false, err
	}
	encBytes = append([]byte(acl), encryptedValue...)
	err = a.store.Put(a.ctx, ds.NewKey(origin), encBytes)
	if err != nil {
		logger.Error(err)
		return false, err
	}
	return true, nil
}

func (a *App) secureGet(origin string, originSelf string) (string, error) {
	// Retrieve the value from the store
	value, err := a.store.Get(a.ctx, ds.NewKey(origin))
	if err != nil {
		logger.Error(err)
		return "", err

	}
	//check ACL
	if checkACL(string(value[:2]), "1", originSelf, origin) {
		return "", nil
	}
	// Decrypt the value with the private key
	decryptedValue, err := AesGCMDecrypt(a.dbCryptKey, value[2:]) //chop acl off
	if err != nil {
		logger.Error(err)
		return "", err
	}

	return string(decryptedValue), nil
}
