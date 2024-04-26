package main

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	ds "github.com/ipfs/go-datastore"
	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/jaketrock/zome/zcrypto"
	"github.com/lucsky/cuid"
)

func (a *App) putObjectRoute(wc peerConn, request []byte, originSelf string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.PutObjectRequest `json:"data"`
	}
	var successObj = sharedInterfaces.PutObjectResponse{DidSucceed: false}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if requestBody.Data.FileName == "" {
		wc.sendMessage(400, fmtError("file name is required"))
		return
	}

	cleanMacl, err := zcrypto.SanitizeACL(requestBody.Data.ACL)
	if err != nil {
		wc.sendMessage(400, fmtError("File Metadata ACL format error "+err.Error()))
		return
	}
	cleanFACL, err := zcrypto.SanitizeACL(requestBody.Data.FACL)
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
		if !zcrypto.CheckACL(priorValue.ACL, "2", originSelf, origin) {
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

	a.fsActiveWrites[randomId] = sharedInterfaces.UploadHeader{
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

func (a *App) getObjectRoute(wc peerConn, request []byte, originSelf string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.GetObjectRequest `json:"data"`
	}
	var successObj = sharedInterfaces.GetObjectResponse{DidSucceed: false}

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
		if !zcrypto.CheckACL(priorValue.FACL, "1", originSelf, origin) {
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

	a.fsActiveReads[randomId] = sharedInterfaces.DownloadHeader{
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

func (a *App) deleteObjectRoute(wc peerConn, request []byte, originSelf string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.DeleteObjectRequest `json:"data"`
	}
	var successObj = sharedInterfaces.DeleteObjectResponse{DidSucceed: false}

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
		if !zcrypto.CheckACL(priorValue.ACL, "3", originSelf, origin) {
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

func (a *App) setGlobalFACL(wc peerConn, request []byte, selfOrigin string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.SetGlobalFACLRequest `json:"data"`
	}

	err := json.Unmarshal(request, &requestBody)
	if err != nil {
		wc.sendMessage(400, fmtError(err.Error()))
	}

	if !requestBody.Data.Value && requestBody.Data.Value {
		wc.sendMessage(400, fmtError("invalid request body: value must be true or false"))
		return
	}

	success := sharedInterfaces.SetGlobalFACLResponse{
		DidSucceed: false,
	}

	origin := selfOrigin + "]-FACL"

	enableByte := []byte{0}
	if requestBody.Data.Value {
		enableByte = []byte{1}
	}

	err = a.secureInternalKeyAdd(origin, enableByte)
	if err != nil {
		success.DidSucceed = false
		wc.sendMessage(500, fmtError("error storing GFACL "+err.Error()))
	} else {
		success.DidSucceed = true
	}

	wc.sendMessage(200, success)
}

func (a *App) getGlobalFACL(wc peerConn, _ []byte, selfOrigin string) {
	gwrite, err := a.globalWriteAbstract(selfOrigin, "FACL")
	if err != nil {
		wc.sendMessage(500, fmtError("error getting global FACL "+err.Error()))
		return
	}

	successObj := sharedInterfaces.GetGlobalFACLResponse{
		GlobalFsAccess: gwrite,
	}

	wc.sendMessage(200, successObj)
}

func (a *App) getDirectoryListing(wc peerConn, request []byte, selfOrigin string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.GetDirectoryListingRequest `json:"data"`
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

	returnMessage := sharedInterfaces.GetDirectoryListingResponse{
		DidSucceed: true,
		Files:      metaObject,
	}

	wc.sendMessage(200, returnMessage)
}

func (a *App) removeObjectOrigin(wc peerConn, request []byte, selfOrigin string) {
	var requestBody struct {
		sharedInterfaces.Request
		Data sharedInterfaces.RemoveObjectOriginRequest `json:"data"`
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
	if !zcrypto.CheckACL(selfOrigin, "3", selfOrigin, selfOrigin) {
		wc.sendMessage(403, fmtError("remove permission denied"))
		return
	}

	// remove origin
	err = a.store.Delete(a.ctx, ds.NewKey(requestBody.Data.Directory))
	if err != nil {
		wc.sendMessage(500, fmtError("error removing directory from db "+err.Error()))
		return
	}

	//remove fs directory
	key := path.Join(selfOrigin, requestBody.Data.Directory)
	delPath := path.Join(a.operatingPath, "zome", "data", key)

	err = os.RemoveAll(delPath)
	if err != nil {
		wc.sendMessage(500, fmtError("error removing directory from fs "+err.Error()))
		return
	}

	wc.sendMessage(200, sharedInterfaces.RemoveObjectOriginResponse{DidSucceed: true})
}

func illegalFileName(path string) bool {
	return (strings.Contains(path, "..") ||
		strings.Contains(path, "~"))
}
