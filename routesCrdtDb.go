package main

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
)

func (a *App) removeOrigin(_ *websocket.Conn, _ Request, originKey string) ([]byte, error) {
	type successReturn struct {
		DidSucceed bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: false,
	}
	err := a.store.DB.DropPrefix([]byte(originKey))
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

func (a *App) setGlobalWrite(_ *websocket.Conn, request Request, originKey string) ([]byte, error) {
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

	origin := originKey + "]-GW"

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

func (a *App) getGlobalWrite(origin string) (bool, error) {
	originKey := origin + "]-GW"
	value, err := a.store.Get(a.ctx, ds.NewKey(originKey))
	if err != nil {
		if err != ds.ErrNotFound {
			return false, err
		} else {
			//repair db to secure state
			err = a.store.Put(a.ctx, ds.NewKey(originKey+"]-GW"), []byte{0})
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

func (a *App) handleAddRequest(_ *websocket.Conn, request Request, originKey string) ([]byte, error) { //TODO: switch to using badger write
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

	success := successReturn{
		DidSucceed: make(map[string]bool, len(requestBody.Values)),
	}

	acl, err := sanitizeACL(requestBody.ACL)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	globalWrite, err := a.getGlobalWrite(originKey)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	for k, v := range requestBody.Values {
		origin := originKey + "-" + k
		if request.ForceDomain != "" {
			origin = request.ForceDomain + "-" + k
		}
		// check value exists
		value, err := a.store.Get(a.ctx, ds.NewKey(origin))
		if err != nil {
			if err != ds.ErrNotFound {
				success.DidSucceed[k] = false
				continue
			} else if globalWrite { // only continue if the global write is enabled
				success.DidSucceed[k] = false
			}
		} else if checkACL(string(value[:2]), "2", originKey, origin) {
			success.DidSucceed[k] = false
			continue
		}

		var encBytes []byte
		// Encrypt the value with the private key
		encryptedValue, err := AesGCMEncrypt(a.dbCryptKey, []byte(v))
		if err != nil {
			logger.Error(err)
			return nil, err
		}
		encBytes = append([]byte(acl), encryptedValue...)
		err = a.store.Put(a.ctx, ds.NewKey(origin), encBytes)
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

func (a *App) handleGetRequest(_ *websocket.Conn, request Request, originKey string) ([]byte, error) {
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

	returnMessages := returnMessage{
		Success: false,
		Keys:    make(map[string]string, len(requestBody.Values)),
	}

	for _, k := range requestBody.Values {
		origin := originKey + "-" + k
		if request.ForceDomain != "" {
			origin = request.ForceDomain + "-" + k
		}
		// Retrieve the value from the store
		value, err := a.store.Get(a.ctx, ds.NewKey(origin))
		if err != nil {
			logger.Error(err)
			returnMessages.Keys[k] = "error"
			continue
		}
		//check ACL
		if checkACL(string(value[:2]), "1", originKey, origin) {
			returnMessages.Keys[k] = "Bad ACL"
			continue
		}
		// Decrypt the value with the private key
		decryptedValue, err := AesGCMDecrypt(a.dbCryptKey, value[2:]) //chop acl off
		if err != nil {
			logger.Error(err)
			returnMessages.Keys[k] = "error"
		}
		returnMessages.Keys[k] = string(decryptedValue)
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

func (a *App) handleDeleteRequest(_ *websocket.Conn, request Request, originKey string) ([]byte, error) {
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
		origin := originKey + "-" + k
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
		if checkACL(string(value[:2]), "3", originKey, origin) {
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
