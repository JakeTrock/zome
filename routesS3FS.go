package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
	"github.com/lucsky/cuid"
)

func (a *App) PutObjectRoute(conn *websocket.Conn, request Request, originKey string) ([]byte, error) {
	var requestBody struct { // TODO: add forcedomain(also should we cache global ACLs?)
		FileName     string `json:"filename"` //TODO: change filename to key
		FileSize     int64  `json:"filesize"`
		Tagging      string `json:"tagging"`
		OverridePath string `json:"overridePath"`
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		UploadId   string `json:"uploadId"` //the id which you will open a socket to in /upload/:id
		Error      string `json:"error"`
	}{DidSucceed: false}

	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		successObj.Error = err.Error()
	}

	if requestBody.FileName == "" {
		return nil, fmt.Errorf("file name is required")
	}
	//TODO: sanitize path, ensure not contains .., or isn't a dir
	if requestBody.FileSize == 0 {
		return nil, fmt.Errorf("file size is required")
	}

	key := path.Join(originKey, requestBody.FileName)
	//check file not exists
	writePath := path.Join(a.operatingPath, "zome", "data", key)
	_, err = os.Stat(writePath)
	if err == nil {
		return nil, fmt.Errorf("file already exists: %s", key)
	}

	randomId := cuid.New()

	a.fsActiveWrites[randomId] = UploadHeader{
		Filename: requestBody.FileName,
		Size:     requestBody.FileSize,
	}

	successObj.UploadId = randomId

	metaObject := make(map[string]string)
	if requestBody.Tagging != "" {
		u, err := url.Parse("/?" + requestBody.Tagging)
		if err != nil {
			return nil, fmt.Errorf("enable to parse tagging string %q: %w", requestBody.Tagging, err)
		}

		q := u.Query()
		for k := range q {
			metaObject[k] = q.Get(k)
		}
	}

	metaObjJson, err := json.Marshal(metaObject)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	origin := originKey
	if request.ForceDomain != "" {
		origin = request.ForceDomain
	}

	writeObject := make(map[string]string)
	writeObject[requestBody.FileName] = string(metaObjJson)

	a.secureAddLoop(writeObject, "33", origin, originKey)

	successObj.DidSucceed = true

	sbytes, err := json.Marshal(successObj)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	return sbytes, nil
}

func (a *App) GetObjectRoute(conn *websocket.Conn, request Request, originKey string) ([]byte, error) {
	var requestBody struct {
		Key string `json:"key"`
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		Error      string `json:"error"`
		MetaData   string `json:"metadata"`
		DownloadId string `json:"downloadId"`
	}{DidSucceed: false}

	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		successObj.Error = err.Error()
	}

	if requestBody.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	key := path.Join(originKey, requestBody.Key)
	writePath := path.Join(a.operatingPath, "zome", "data", key)

	_, err = os.Stat(writePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("the specified file key does not exist: %s", key)
		}
		return nil, err
	}

	metaObject, err := a.secureGetLoop([]string{requestBody.Key}, originKey, originKey)
	// metaObject, err := a.store.Get(a.ctx, ds.NewKey(requestBody.Key))
	if err != nil {
		return nil, fmt.Errorf("error getting metadata for key %q: %w", key, err)
	}
	// metaJson := make(map[string]string)
	// err = json.Unmarshal(metaObject, &metaJson)
	// if err != nil {
	// 	return nil, err
	// }

	randomId := cuid.New()

	a.fsActiveReads[randomId] = requestBody.Key

	successObj.DidSucceed = true
	successObj.MetaData = metaObject[requestBody.Key]
	successObj.DownloadId = randomId
	sbytes, err := json.Marshal(successObj)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	return sbytes, nil
}

func (a *App) DeleteObjectRoute(conn *websocket.Conn, request Request, originKey string) ([]byte, error) {
	var requestBody struct {
		Key string `json:"key"`
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		Error      string `json:"error"`
	}{DidSucceed: false}

	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		successObj.Error = err.Error()
	}

	if requestBody.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	key := path.Join(originKey, requestBody.Key)
	writePath := path.Join(a.operatingPath, "zome", "data", key)

	err = os.Remove(writePath)
	if errors.Is(err, os.ErrNotExist) {
		successObj.DidSucceed = false
	}
	if err != nil {
		return nil, err
	}

	err = a.store.Delete(a.ctx, ds.NewKey(key))
	if err == ds.ErrNotFound {
		successObj.DidSucceed = false
	}
	if err != nil {
		return nil, err
	}

	successObj.DidSucceed = true

	sbytes, err := json.Marshal(successObj)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	return sbytes, nil
}
