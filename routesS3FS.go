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
)

func (a *App) PutObjectRoute(conn *websocket.Conn, request Request, originKey string) ([]byte, error) {
	var requestBody struct {
		Key     string `json:"key"`
		Tagging string `json:"tagging"`
		file    io.Reader
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		Hash       string `json:"hash"`
		Error      string `json:"error"`
	}{DidSucceed: false}

	err := json.Unmarshal(request.Data, &requestBody)
	if err != nil {
		logger.Error(err)
		successObj.Error = err.Error()
	}

	if requestBody.Key == "" {
		return nil, fmt.Errorf("Key is required")
	}

	a.fsMutex.Lock()
	defer a.fsMutex.Unlock()

	key := path.Join(originKey, requestBody.Key)
	writePath := path.Join(a.operatingPath, "data", key)

	file, err := os.Create(writePath) // TODO: add forcedomain(also should we cache global ACLs?)

	if err != nil {
		logger.Error(err)
		return nil, err
	}
	defer file.Close()

	_, err = io.Copy(file, requestBody.file)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	metaObject := map[string]string{}
	if requestBody.Tagging != "" {
		u, err := url.Parse("/?" + requestBody.Tagging)
		if err != nil {
			panic(fmt.Errorf("Unable to parse tagging string %q: %w", requestBody.Tagging, err))
		}

		q := u.Query()
		for k := range q {
			metaObject[k] = q.Get(k)
		}
	}

	//calculate file sha256
	sum256, err := sha256File(writePath)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	metaObject["sha256"] = sum256

	metaObjJson, err := json.Marshal(metaObject)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	err = a.store.Put(a.ctx, ds.NewKey(key), metaObjJson)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	successObj.DidSucceed = true
	successObj.Hash = sum256
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
		return nil, fmt.Errorf("Key is required")
	}

	a.fsMutex.Lock()
	defer a.fsMutex.Unlock()

	key := path.Join(originKey, requestBody.Key)
	writePath := path.Join(a.operatingPath, "data", key)

	_, err = os.Stat(writePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("The specified key does not exist: %s", key)
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

	metaObject, err := a.store.Get(a.ctx, ds.NewKey(key))
	if err != nil {
		return nil, err
	}
	metaJson := map[string]string{}
	err = json.Unmarshal(metaObject, &metaJson)
	if err != nil {
		return nil, err
	}

	metaJsonPtr := make(map[string]*string)
	for k, v := range metaJson {
		value := v
		metaJsonPtr[k] = &value
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
		return nil, fmt.Errorf("Key is required")
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
