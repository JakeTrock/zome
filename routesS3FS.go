package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path"

	ds "github.com/ipfs/go-datastore"
	"github.com/lucsky/cuid"
)

func (a *App) PutObjectRoute(wc wsConn, request []byte, originKey string) {
	var requestBody struct { // TODO: add forcedomain(also should we cache global ACLs?)
		Request
		Data struct {
			FileName     string `json:"filename"` //TODO: change filename to key
			FileSize     int64  `json:"filesize"`
			Tagging      string `json:"tagging"`
			OverridePath string `json:"overridePath"`
		} `json:"data"`
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		UploadId   string `json:"uploadId"` //the id which you will open a socket to in /upload/:id
	}{DidSucceed: false}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
	}

	if requestBody.Data.FileName == "" {
		wc.sendMessage(400, ("file name is required"))
		return
	}
	//TODO: sanitize path, ensure not contains .., or isn't a dir
	if requestBody.Data.FileSize == 0 {
		wc.sendMessage(400, ("file size is required"))
		return
	}

	key := path.Join(originKey, requestBody.Data.FileName)
	//check file not exists
	writePath := path.Join(a.operatingPath, "zome", "data", key)
	_, err = os.Stat(writePath)
	if err == nil {
		wc.sendMessage(400, ("file already exists"))
		return
	}

	randomId := cuid.New()

	a.fsActiveWrites[randomId] = UploadHeader{
		Filename: requestBody.Data.FileName,
		Size:     requestBody.Data.FileSize,
	}

	successObj.UploadId = randomId

	metaObject := make(map[string]string)
	if requestBody.Data.Tagging != "" {
		u, err := url.Parse("/?" + requestBody.Data.Tagging)
		if err != nil {
			wc.sendMessage(400, ("error parsing tagging string"))
			return
		}

		q := u.Query()
		for k := range q {
			metaObject[k] = q.Get(k)
		}
	}

	metaObjJson, err := json.Marshal(metaObject)
	if err != nil {
		wc.sendMessage(500, ("error marshalling metadata"))
		return
	}

	origin := originKey
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}

	writeObject := make(map[string]string)
	writeObject[requestBody.Data.FileName] = string(metaObjJson)

	a.secureAddLoop(writeObject, "33", origin, originKey)

	successObj.DidSucceed = true

	wc.sendMessage(200, successObj)
	return
}

func (a *App) GetObjectRoute(wc wsConn, request []byte, originKey string) {
	var requestBody struct {
		Request
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		MetaData   string `json:"metadata"`
		DownloadId string `json:"downloadId"`
	}{DidSucceed: false}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
		return
	}

	if requestBody.Data.Key == "" {
		wc.sendMessage(400, ("key is required"))
		return
	}

	key := path.Join(originKey, requestBody.Data.Key)
	writePath := path.Join(a.operatingPath, "zome", "data", key)

	_, err = os.Stat(writePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			wc.sendMessage(400, ("the specified file key does not exist"))
			return
		}
		wc.sendMessage(500, ("error checking file existence"))
		return
	}

	metaObject, err := a.secureGetLoop([]string{requestBody.Data.Key}, originKey, originKey)
	// metaObject, err := a.store.Get(a.ctx, ds.NewKey(requestBody.Key))
	if err != nil {
		wc.sendMessage(500, ("error getting metadata for key " + requestBody.Data.Key + ": " + err.Error()))
		return
	}
	// metaJson := make(map[string]string)
	// err = json.Unmarshal(metaObject, &metaJson)
	// if err != nil {
	// 	return nil, err
	// }

	randomId := cuid.New()

	a.fsActiveReads[randomId] = requestBody.Data.Key

	successObj.DidSucceed = true
	successObj.MetaData = metaObject[requestBody.Data.Key]
	successObj.DownloadId = randomId
	wc.sendMessage(200, successObj)
	return
}

func (a *App) DeleteObjectRoute(wc wsConn, request []byte, originKey string) {
	var requestBody struct {
		Request
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	var successObj = struct {
		DidSucceed bool `json:"didSucceed"`
	}{DidSucceed: false}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, (err.Error()))
		return
	}

	if requestBody.Data.Key == "" {
		wc.sendMessage(400, ("key is required"))
		return
	}

	key := path.Join(originKey, requestBody.Data.Key)
	writePath := path.Join(a.operatingPath, "zome", "data", key)

	err = os.Remove(writePath)
	if errors.Is(err, os.ErrNotExist) {
		successObj.DidSucceed = false
	}
	if err != nil {
		wc.sendMessage(500, ("error deleting file: " + err.Error()))
		return
	}

	err = a.store.Delete(a.ctx, ds.NewKey(key))
	if err == ds.ErrNotFound {
		successObj.DidSucceed = false
	}
	if err != nil {
		wc.sendMessage(500, ("error deleting metadata: " + err.Error()))
		return
	}

	successObj.DidSucceed = true

	wc.sendMessage(200, successObj)
	return
}
