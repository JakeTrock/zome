package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
	"github.com/lucsky/cuid"
)

func (a *App) PutObjectRoute(conn *websocket.Conn, request Request, originKey string) ([]byte, error) {
	var requestBody struct { // TODO: add forcedomain(also should we cache global ACLs?)
		FileName     string `json:"filename"`
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
	writePath := path.Join(a.operatingPath, "data", key)
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
	writeObject[key] = string(metaObjJson)

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
		Key     string `json:"key"`
		Tagging string `json:"tagging"`
		file    io.Reader
	}
	var successObj = struct {
		DidSucceed bool              `json:"didSucceed"`
		Error      string            `json:"error"`
		MetaData   map[string]string `json:"metadata"`
		Body       io.ReadCloser     `json:"body"`
	}{DidSucceed: false}

	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		successObj.Error = err.Error()
	}

	if requestBody.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	a.fsMutex.Lock()
	defer a.fsMutex.Unlock()

	key := path.Join(originKey, requestBody.Key)
	writePath := path.Join(a.operatingPath, "data", key)

	_, err = os.Stat(writePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("the specified key does not exist: %s", key)
		}
		return nil, err
	}

	//open file
	file, err := os.Open(writePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	metaObject, err := a.store.Get(a.ctx, ds.NewKey(key))
	if err != nil {
		return nil, err
	}
	metaJson := make(map[string]string)
	err = json.Unmarshal(metaObject, &metaJson)
	if err != nil {
		return nil, err
	}

	successObj.DidSucceed = true
	successObj.MetaData = metaJson
	successObj.Body = io.NopCloser(bytes.NewReader(fileBytes)) //TODO: send as websocket loop
	sbytes, err := json.Marshal(successObj)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	return sbytes, nil
}

func (a *App) DeleteObjectRoute(conn *websocket.Conn, request Request, originKey string) ([]byte, error) {
	var requestBody struct {
		Key     string `json:"key"`
		Tagging string `json:"tagging"`
		file    io.Reader
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

	a.fsMutex.Lock()
	defer a.fsMutex.Unlock()

	key := path.Join(originKey, requestBody.Key)
	writePath := path.Join(a.operatingPath, "data", key)

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
