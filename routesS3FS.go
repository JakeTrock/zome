package main

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	ds "github.com/ipfs/go-datastore"
	"github.com/lucsky/cuid"
)

func (a *App) putObjectRoute(wc wsConn, request []byte, originSelf string) {
	var requestBody struct {
		Request
		Data struct {
			FileName     string            `json:"filename"`
			FileSize     int64             `json:"filesize"`
			Tagging      map[string]string `json:"tagging"`
			OverridePath string            `json:"overridePath"`
			ACL          string            `json:"acl"`  // acl of the metadata
			FACL         string            `json:"facl"` // acl of the file within metadata
			Encryption   bool              `json:"encryption"`
		} `json:"data"`
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		UploadId   string `json:"uploadId"` //the id which you will open a socket to in /upload/:id
		Error      string `json:"error"`
	}{DidSucceed: false}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if requestBody.Data.FileName == "" {
		wc.sendMessage(400, fmtError("file name is required"))
		return
	}

	cleanMacl, err := sanitizeACL(requestBody.Data.ACL)
	if err != nil {
		wc.sendMessage(400, fmtError("File Metadata ACL format error "+err.Error()))
		return
	}
	cleanFACL, err := sanitizeACL(requestBody.Data.FACL)
	if err != nil {
		wc.sendMessage(400, fmtError("File ACL format error "+err.Error()))
		return
	}

	// Ensure file path doesn't contain ".."
	if illegalFileName(requestBody.Data.FileName) {
		wc.sendMessage(400, fmtError("invalid file path"))
		return
	}

	if requestBody.Data.FileSize == 0 {
		wc.sendMessage(400, fmtError("file size is required"))
		return
	}

	//see if cross origin, see if allowed
	origin := originSelf
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}
	//see if file metadata exists
	priorMetaData, error := a.secureGet(requestBody.Data.FileName, origin, originSelf)
	if error != nil {
		wc.sendMessage(500, fmtError("error getting prior metadata "+error.Error()))
		return
	}
	if priorMetaData != "" {
		//unmarshal priorMetaData
		var priorValue struct {
			ACL string `json:"ACL"`
		}
		err = json.Unmarshal([]byte(priorMetaData), &priorValue)
		if err != nil {
			wc.sendMessage(500, fmtError(err.Error()))
			return
		}
		//check data ACL against own domain, ignores meta ACL
		if !checkACL(priorValue.ACL, "2", originSelf, origin) {
			wc.sendMessage(403, fmtError("edit permission denied"))
			return
		}
	} else {
		gfacl, err := a.globalWriteAbstract(origin, "FACL")
		if err != nil {
			wc.sendMessage(500, fmtError(err.Error()))
			return
		}
		if !gfacl {
			wc.sendMessage(403, fmtError("global write permission denied"))
			return
		}
	}

	key := path.Join(origin, requestBody.Data.FileName)
	//check file not exists
	writePath := path.Join(a.operatingPath, "zome", "data", key)
	_, err = os.Stat(writePath)
	if err == nil {
		wc.sendMessage(400, fmtError("file already exists"))
		return
	}

	randomId := cuid.New()

	a.fsActiveWrites[randomId] = UploadHeader{
		Filename:   requestBody.Data.FileName,
		Size:       requestBody.Data.FileSize,
		Domain:     origin,
		Encryption: requestBody.Data.Encryption,
	}

	successObj.UploadId = randomId

	requestBody.Data.Tagging["FACL"] = cleanFACL
	taggingJson, err := json.Marshal(requestBody.Data.Tagging)
	if err != nil {
		wc.sendMessage(500, fmtError("error marshalling tagging"))
		return
	}

	successObj.DidSucceed, err = a.secureAdd(
		requestBody.Data.FileName,
		string(taggingJson),
		cleanMacl,
		origin,
		originSelf,
		false)
	if err != nil {
		wc.sendMessage(500, fmtError("error adding metadata "+err.Error()))
		return
	}

	wc.sendMessage(200, successObj)
}

func (a *App) getObjectRoute(wc wsConn, request []byte, originSelf string) {
	var requestBody struct {
		Request
		Data struct {
			FileName     string `json:"fileName"`
			ContinueFrom int64  `json:"continueFrom"`
			ForceDomain  string `json:"forceDomain"`
			Encryption   bool   `json:"encryption"`
		} `json:"data"`
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		MetaData   string `json:"metadata"`
		DownloadId string `json:"downloadId"`
	}{DidSucceed: false}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
		return
	}

	if requestBody.Data.FileName == "" {
		wc.sendMessage(400, fmtError("filename is required"))
		return
	}

	if illegalFileName(requestBody.Data.FileName) {
		wc.sendMessage(400, fmtError("invalid file path"))
		return
	}

	if requestBody.Data.ContinueFrom > 0 && requestBody.Data.Encryption {
		wc.sendMessage(400, fmtError("cannot use continue from an encrypted file"))
		return
	}

	origin := originSelf
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}

	priorMetaData, error := a.secureGet(requestBody.Data.FileName, origin, originSelf)
	if error != nil {
		wc.sendMessage(500, fmtError("error getting object "+error.Error()))
		return
	}
	if priorMetaData != "" {
		//unmarshal priorMetaData
		var priorValue struct {
			FACL string `json:"FACL"`
		}

		err = json.Unmarshal([]byte(priorMetaData), &priorValue)
		if err != nil {
			wc.sendMessage(500, fmtError("error deserializing object metadata "+err.Error()))
			return
		}
		//check data ACL against own domain, ignores meta ACL
		if !checkACL(priorValue.FACL, "1", originSelf, origin) {
			wc.sendMessage(403, fmtError("read permission denied"))
			return
		}
	} else {
		wc.sendMessage(403, fmtError("no metadata for file"))
		return
	}

	key := path.Join(origin, requestBody.Data.FileName)
	writePath := path.Join(a.operatingPath, "zome", "data", key)

	_, err = os.Stat(writePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			wc.sendMessage(400, fmtError("the specified file key does not exist"))
			return
		}
		wc.sendMessage(500, fmtError("error checking file existence"))
		return
	}

	metaObject, err := a.secureGetLoop([]string{requestBody.Data.FileName}, origin, originSelf)
	if err != nil {
		wc.sendMessage(500, fmtError("error getting metadata for key "+requestBody.Data.FileName+": "+err.Error()))
		return
	}

	randomId := cuid.New()

	a.fsActiveReads[randomId] = DownloadHeader{
		Filename:     requestBody.Data.FileName,
		ContinueFrom: requestBody.Data.ContinueFrom,
		Domain:       origin,
		Encryption:   requestBody.Data.Encryption,
	}

	successObj.DidSucceed = true
	successObj.MetaData = metaObject[requestBody.Data.FileName]
	successObj.DownloadId = randomId
	wc.sendMessage(200, successObj)
}

func (a *App) deleteObjectRoute(wc wsConn, request []byte, originSelf string) {
	var requestBody struct {
		Request
		Data struct {
			FileName string `json:"fileName"`
		} `json:"data"`
	}
	var successObj = struct {
		DidSucceed bool   `json:"didSucceed"`
		Error      string `json:"error"`
	}{DidSucceed: false}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
		return
	}

	if requestBody.Data.FileName == "" {
		wc.sendMessage(400, fmtError("filename is required"))
		return
	}

	if illegalFileName(requestBody.Data.FileName) {
		wc.sendMessage(400, fmtError("invalid file path"))
		return
	}

	origin := originSelf
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}
	priorMetaData, error := a.secureGet(requestBody.Data.FileName, origin, originSelf)
	if error != nil {
		wc.sendMessage(500, fmtError("error getting metadata for deletion "+error.Error()))
		return
	}
	if priorMetaData != "" {
		//unmarshal priorMetaData
		var priorValue struct {
			ACL string `json:"ACL"`
		}
		err = json.Unmarshal([]byte(priorMetaData), &priorValue)
		if err != nil {
			wc.sendMessage(500, fmtError("error deserializing metadata for deletion"+err.Error()))
			return
		}
		//check data ACL against own domain, ignores meta ACL
		if !checkACL(priorValue.ACL, "3", originSelf, origin) {
			wc.sendMessage(403, fmtError("delete permission denied"))
			return
		}
	} else {
		wc.sendMessage(403, fmtError("no metadata for file"))
		return
	}

	key := path.Join(origin, requestBody.Data.FileName)
	writePath := path.Join(a.operatingPath, "zome", "data", key)

	err = os.Remove(writePath)
	if errors.Is(err, os.ErrNotExist) {
		successObj.DidSucceed = false
	}
	if err != nil {
		wc.sendMessage(500, fmtError("error deleting file: "+err.Error()))
		return
	}

	err = a.store.Delete(a.ctx, ds.NewKey(key))
	if err == ds.ErrNotFound {
		successObj.DidSucceed = false
		successObj.Error = "metadata not found"
	}
	if err != nil {
		wc.sendMessage(500, fmtError("error deleting metadata: "+err.Error()))
		return
	}

	successObj.DidSucceed = true

	wc.sendMessage(200, successObj)
}

func (a *App) setGlobalFACL(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Request
		Data struct {
			Value bool `json:"value"`
		} `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if !requestBody.Data.Value && requestBody.Data.Value {
		wc.sendMessage(400, fmtError("invalid request body: value must be true or false"))
		return
	}

	type successReturn struct {
		DidSucceed bool `json:"didSucceed"`
	}

	success := successReturn{
		DidSucceed: false,
	}

	origin := selfOrigin + "]-FACL"

	enableByte := []byte{0}
	if requestBody.Data.Value {
		enableByte = []byte{1}
	}

	err = a.store.Put(a.ctx, ds.NewKey(origin), enableByte)
	if err != nil {
		success.DidSucceed = false
		wc.sendMessage(500, fmtError("error storing GFACL "+err.Error()))
	} else {
		success.DidSucceed = true
	}

	wc.sendMessage(200, success)
}

func (a *App) getGlobalFACL(wc wsConn, _ []byte, selfOrigin string) {
	gwrite, err := a.globalWriteAbstract(selfOrigin, "FACL")
	if err != nil {
		wc.sendMessage(500, fmtError("error getting global FACL "+err.Error()))
		return
	}

	successObj := struct {
		GlobalFsAccess bool `json:"globalFsAccess"`
	}{
		GlobalFsAccess: gwrite,
	}

	wc.sendMessage(200, successObj)
}

func (a *App) getDirectoryListing(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Request
		Data struct {
			Directory string `json:"directory"`
		} `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if requestBody.Data.Directory == "" {
		wc.sendMessage(400, fmtError("directory is required"))
		return
	}

	if illegalFileName(requestBody.Data.Directory) {
		wc.sendMessage(400, fmtError("invalid directory path"))
		return
	}

	origin := selfOrigin
	if requestBody.ForceDomain != "" {
		origin = requestBody.ForceDomain
	}
	// see if path exists
	key := path.Join(origin, requestBody.Data.Directory)
	readPath := path.Join(a.operatingPath, "zome", "data", key)

	_, err = os.Stat(readPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			wc.sendMessage(400, fmtError("the specified directory does not exist"))
			return
		}
		wc.sendMessage(500, fmtError("error checking directory existence"))
		return
	}

	directoryListing := make([]string, 0)
	//get directory listing
	err = filepath.Walk(readPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		directoryListing = append(directoryListing, path)
		return nil
	})

	if err != nil {
		wc.sendMessage(500, fmtError("error getting directory listing "+err.Error()))
		return
	}

	//remove the directory path from the listing
	directoryListing = directoryListing[1:]

	// remove the base path from all listings
	for i, v := range directoryListing {
		tstring := strings.TrimPrefix(v, readPath)
		// remove leading slash
		directoryListing[i] = path.Join(requestBody.Data.Directory, tstring)
	}

	// get metadata for each file
	metaObject, err := a.secureGetLoop(directoryListing, origin, selfOrigin)
	if err != nil {
		wc.sendMessage(500, fmtError("error getting metadata for directory listing "+err.Error()))
		return
	}

	returnMessage := struct {
		DidSucceed bool              `json:"didSucceed"`
		Files      map[string]string `json:"files"`
	}{
		DidSucceed: true,
		Files:      metaObject,
	}

	wc.sendMessage(200, returnMessage)
}

func (a *App) removeObjectOrigin(wc wsConn, request []byte, selfOrigin string) {
	var requestBody struct {
		Request
		Data struct {
			Directory string `json:"directory"`
		} `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if requestBody.Data.Directory == "" {
		wc.sendMessage(400, fmtError("directory is required"))
		return
	}

	// check if origin is allowed to be removed
	if !checkACL(selfOrigin, "3", selfOrigin, selfOrigin) {
		wc.sendMessage(403, fmtError("remove permission denied"))
		return
	}

	// remove origin
	err = a.store.Delete(a.ctx, ds.NewKey(requestBody.Data.Directory))
	if err != nil {
		wc.sendMessage(500, fmtError("error removing directory "+err.Error()))
		return
	}

	//remove fs directory
	key := path.Join(selfOrigin, requestBody.Data.Directory)
	delPath := path.Join(a.operatingPath, "zome", "data", key)

	err = os.RemoveAll(delPath)

	wc.sendMessage(200, struct {
		DidSucceed bool `json:"didSucceed"`
	}{true})
}

func illegalFileName(path string) bool {
	return (strings.Contains(path, "..") ||
		strings.Contains(path, "~"))
}
