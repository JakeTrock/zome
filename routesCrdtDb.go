package main

import (
	"encoding/json"

	ds "github.com/ipfs/go-datastore"
	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/jaketrock/zome/zcrypto"
)

func (a *App) handleAddRequest(wc peerConn, request []byte, selfOrigin string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.DbAddRequest `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	//validate request
	if len(requestBody.Data.Values) == 0 {
		wc.sendMessage(400, fmtError("invalid request body: no values provided"))
		return
	}

	if requestBody.Data.ACL == "" {
		requestBody.Data.ACL = "11"
	}

	var origin = selfOrigin
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}

	successResult, err := a.secureAddLoop(requestBody.Data.Values, requestBody.Data.ACL, origin, selfOrigin)
	if err != nil {

		wc.sendMessage(500, fmtError(err.Error()))
		return
	}

	success := sharedInterfaces.DbAddResponse{
		DidSucceed: successResult,
	}

	wc.sendMessage(200, success)
}

func (a *App) handleGetRequest(wc peerConn, request []byte, selfOrigin string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.DbGetRequest `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if len(requestBody.Data.Values) == 0 {
		wc.sendMessage(400, fmtError("invalid request body: no keys provided"))
		return
	}

	var origin = selfOrigin
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}

	getResult, err := a.secureGetLoop(requestBody.Data.Values, origin, selfOrigin)
	if err != nil {
		wc.sendMessage(500, fmtError(err.Error()))
		return
	}

	returnMessages := sharedInterfaces.DbGetResponse{
		Success: false,
		Keys:    getResult,
	}

	returnMessages.Success = true

	wc.sendMessage(200, returnMessages)
}

func (a *App) handleDeleteRequest(wc peerConn, request []byte, selfOrigin string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.DbDeleteRequest `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if len(requestBody.Data.Values) == 0 {
		wc.sendMessage(400, fmtError("invalid request body: no keys provided"))
		return
	}

	success := sharedInterfaces.DbDeleteResponse{
		DidSucceed: make(map[string]bool, len(requestBody.Data.Values)),
	}

	for _, k := range requestBody.Data.Values {
		origin := selfOrigin
		if requestBody.ForceDomain != "" {
			origin = requestBody.ForceDomain
		}
		// Retrieve the value from the store
		value, err := a.store.Get(a.ctx, ds.NewKey(origin+"-"+k))
		if err != nil {
			success.DidSucceed[k] = false
			success.Error += err.Error() + ", "
			continue
		}
		//check ACL
		if !zcrypto.CheckACL(string(value[:2]), "3", selfOrigin, origin) {
			success.DidSucceed[k] = false
			success.Error += err.Error() + ", "
			continue
		}
		// Delete the key from the store
		err = a.store.Delete(a.ctx, ds.NewKey(origin+"-"+k))
		if err != nil {
			success.DidSucceed[k] = false
			success.Error += err.Error() + ", "
		} else {
			success.DidSucceed[k] = true
		}
	}

	wc.sendMessage(200, success)
}

func (a *App) setGlobalWrite(wc peerConn, request []byte, selfOrigin string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.DbSetGlobalWriteRequest `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if requestBody.Data.Value != true && requestBody.Data.Value != false {
		wc.sendMessage(400, fmtError("invalid request body: value must be true or false"))
		return
	}

	success := sharedInterfaces.DbSetGlobalWriteResponse{
		DidSucceed: false,
	}

	origin := selfOrigin + "]-GW"

	enableByte := []byte{0}
	if requestBody.Data.Value {
		enableByte = []byte{1}
	}
	err = a.secureInternalKeyAdd(origin, enableByte)
	if err != nil {
		success.DidSucceed = false
		wc.sendMessage(500, fmtError(err.Error()))
	} else {
		success.DidSucceed = true
	}

	wc.sendMessage(200, success)
}

func (a *App) getGlobalWrite(wc peerConn, _ []byte, selfOrigin string) {
	gwrite, err := a.globalWriteAbstract(selfOrigin, "GW")
	if err != nil {
		wc.sendMessage(500, fmtError(err.Error()))
		return
	}
	successObj := sharedInterfaces.DbGetGlobalWriteResponse{}
	successObj.GlobalWrite = gwrite

	wc.sendMessage(200, successObj)
}

func (a *App) removeOrigin(wc peerConn, _ []byte, selfOrigin string) {
	success := sharedInterfaces.DbRemoveOriginResponse{
		DidSucceed: false,
	}
	err := a.store.DB.DropPrefix([]byte(selfOrigin))
	if err != nil {
		wc.sendMessage(500, fmtError(err.Error()))
		success.DidSucceed = false
	} else {
		success.DidSucceed = true
	}

	wc.sendMessage(200, success)
}
