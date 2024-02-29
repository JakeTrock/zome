package main

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
)

func (a *App) removeOrigin(conn *websocket.Conn, request Request, originKey string) {
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
		return
	}

	// Send the success response to the client
	err = conn.WriteMessage(websocket.TextMessage, successJson)
	if err != nil {
		logger.Error(err)
		return
	}
}

func (a *App) setGlobalWrite(conn *websocket.Conn, request Request, originKey string) {
	type successReturn struct {
		DidSucceed bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: false,
	}

	origin := originKey + "]-GW"

	enableBool := request.Data.Value == "true"
	enableByte := []byte{0}
	if enableBool {
		enableByte = []byte{1}
	}

	err := a.store.Put(a.ctx, ds.NewKey(origin), enableByte)
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
		return
	}

	// Send the success response to the client
	err = conn.WriteMessage(websocket.TextMessage, successJson)
	if err != nil {
		logger.Error(err)
		return
	}
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

func (a *App) handleAddRequest(conn *websocket.Conn, request Request, originKey string) { //TODO: switch to using badger write
	//validate request
	if len(request.Data.Values) == 0 {
		logger.Error("Invalid request body")
		return
	}

	if request.Data.ACL == "" {
		request.Data.ACL = "11"
	}

	type successReturn struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: make(map[string]bool, len(request.Data.Values)),
	}

	acl, err := sanitizeACL(request.Data.ACL)
	if err != nil {
		logger.Error(err)
		return
	}

	globalWrite, err := a.getGlobalWrite(originKey)
	if err != nil {
		logger.Error(err)
		return
	}

	for k, v := range request.Data.Values {
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
			return
		}
		print("origin: " + origin + "\n")
		encBytes = append([]byte(acl), encryptedValue...)
		print("encBytes: " + string(encBytes) + "\n")
		print("ff: " + string(encBytes[:2]) + "\n")
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
		return
	}

	// Send the success response to the client
	err = conn.WriteMessage(websocket.TextMessage, successJson)
	if err != nil {
		logger.Error(err)
		return
	}
}

func (a *App) handleGetRequest(conn *websocket.Conn, request Request, originKey string) {
	type returnMessage struct {
		Keys map[string]string `json:"keys"`
	}

	returnMessages := returnMessage{
		Keys: make(map[string]string, len(request.Data.Keys)),
	}

	for _, k := range request.Data.Keys {
		origin := originKey + "-" + k
		if request.ForceDomain != "" {
			origin = request.ForceDomain + "-" + k
			print("forced")
		}
		print("origin: " + origin + "\n")
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
	retMessages, err := json.Marshal(returnMessages)
	if err != nil {
		logger.Error(err)
		return
	}

	// Send the return messages to the client
	err = conn.WriteMessage(websocket.TextMessage, retMessages)
	if err != nil {
		logger.Error(err)
		return
	}
}

func (a *App) handleDeleteRequest(conn *websocket.Conn, request Request, originKey string) {
	type successReturn struct {
		DidSucceed map[string]bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: make(map[string]bool, len(request.Data.Keys)),
	}

	for _, k := range request.Data.Keys {
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
		return
	}

	// Send the success response to the client
	err = conn.WriteMessage(websocket.TextMessage, successJson)
	if err != nil {
		logger.Error(err)
		return
	}
}
