package main

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"testing"

	libzome "github.com/jaketrock/zome/libZome"
	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/jaketrock/zome/zcrypto"
	"github.com/stretchr/testify/assert"
)

// TODO: revise these
func (tc *testContext) establishFileUploadSocket(uploadId string) *libzome.ZomeUploader {
	ctpk := tc.app.connTopic.String()
	peerList := tc.libZome.ListPeers(ctpk) //TODO: get server a better way
	ulConn, exists := tc.libZome.ConnectServer(ctpk, peerList[0], libzome.UploadOp).(libzome.ZomeUploader)
	if !exists {
		fmt.Println("could not connect to server for upload")
		return nil
	}
	err := ulConn.InitUpload(uploadId)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return &ulConn
}

func (tc *testContext) establishFileDownloadSocket(downloadId string) *libzome.ZomeDownloader {
	ctpk := tc.app.connTopic.String()
	peerList := tc.libZome.ListPeers(ctpk) //TODO: get server a better way
	dlConn, exists := tc.libZome.ConnectServer(ctpk, peerList[0], libzome.DownloadOp).(libzome.ZomeDownloader)
	if !exists {
		fmt.Println("could not connect to server for upload")
		return nil
	}
	err := dlConn.InitDownload(downloadId)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return &dlConn
}

func (tc *testContext) getTestFile() *os.File {
	file, err := os.OpenFile(path.Join(testPathBase, "./testfile.jpg"), os.O_RDONLY, 0755)
	if err != nil {
		panic(err)
	}
	return file
}

func putGeneralized(t *testing.T, encryption bool) string {
	rawMeta := map[string]string{
		"test1":   zcrypto.GenerateRandomKey(),
		"testTwo": zcrypto.GenerateRandomKey(),
		"from":    "zipdisk",
	}

	testFile := firstApp.getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := zcrypto.GenerateRandomKey()
	fileRequest := sharedInterfaces.PutObjectRequest{
		FileName:     path.Join(randPath, path.Base(testFile.Name())),
		FileSize:     fileSize,
		Tagging:      rawMeta, //this should probly just be nested json
		OverridePath: "",
		ACL:          "11",
		FACL:         "11",
		Encryption:   encryption,
	}

	unmarshalledResponse, err := firstApp.servConn.FsPut(fileRequest)
	assert.NoError(t, err)
	assert.Empty(t, unmarshalledResponse.Error)
	assert.True(t, unmarshalledResponse.DidSucceed)

	uploadId := unmarshalledResponse.UploadId
	assert.NotEmpty(t, uploadId)
	if uploadId == "" {
		return ""
	}
	// init upload socket
	ctrlChannel := make(chan string)
	progressChannel := make(chan int)
	uploadSocket := firstApp.establishFileUploadSocket(uploadId)
	uploadSocket.WriteWholeFile(testFile, ctrlChannel, progressChannel)

	// hashes of input and output match
	ogSha, err := zcrypto.Sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)

	newPath := path.Join(firstApp.path, "zome/data", firstApp.originBase, fileRequest.FileName)
	newSha, err := zcrypto.Sha256File(newPath)
	assert.NoError(t, err)

	if !encryption {
		assert.Equal(t, ogSha, newSha)
	}
	return fileRequest.FileName
}

func createDownloadFile(t *testing.T, fileName string) *os.File {
	// if download folder doesn't exist, make it
	downloadPath := path.Join("testPath", "downloads")
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)

	// create file
	file, err := os.Create(path.Join("testPath", "downloads", fileName))
	assert.NoError(t, err)
	return file
}

func TestPut(t *testing.T) {

	putGeneralized(t, false)
}

func TestFileSeek(t *testing.T) {

	rawMeta := map[string]string{
		"test1":   zcrypto.GenerateRandomKey(),
		"testTwo": zcrypto.GenerateRandomKey(),
		"from":    "zipdisk",
	}

	testFile := firstApp.getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := zcrypto.GenerateRandomKey()
	fileRequest := sharedInterfaces.PutObjectRequest{
		FileName: path.Join(randPath, path.Base(testFile.Name())),
		FileSize: fileSize,
		Tagging:  rawMeta,
		ACL:      "11",
		FACL:     "11",
	}

	unmarshalledResponse, err := firstApp.servConn.FsPut(fileRequest)
	assert.Empty(t, unmarshalledResponse.Error)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)

	uploadId := unmarshalledResponse.UploadId
	assert.NotEmpty(t, uploadId)
	if uploadId == "" {
		return
	}
	//init upload socket
	uploadSocket := firstApp.establishFileUploadSocket(uploadId)
	ctrlChannel := make(chan string)
	progressChannel := make(chan int)

	//TODO: will this work?
	go uploadSocket.WriteWholeFile(testFile, ctrlChannel, progressChannel)

	go func() {
		progress := <-progressChannel
		if progress == 25 {
			fileSize := fileRequest.FileSize
			skipSize := int64(float64(fileSize) * 0.75)
			skipString := strconv.FormatInt(skipSize, 10)
			skipMsg := "SKIPTO" + skipString
			ctrlChannel <- skipMsg
		}
	}()

	//check file is there
	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileRequest.FileName))
	assert.False(t, os.IsNotExist(err))
	//verify that it's only partially populated?
}

func TestFileCancel(t *testing.T) {

	rawMeta := map[string]string{
		"test1":   zcrypto.GenerateRandomKey(),
		"testTwo": zcrypto.GenerateRandomKey(),
		"from":    "zipdisk",
	}

	testFile := firstApp.getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := zcrypto.GenerateRandomKey()
	fileRequest := sharedInterfaces.PutObjectRequest{
		FileName: path.Join(randPath, path.Base(testFile.Name())),
		FileSize: fileSize,
		Tagging:  rawMeta,
		ACL:      "11",
		FACL:     "11",
	}

	unmarshalledResponse, err := firstApp.servConn.FsPut(fileRequest)
	assert.Empty(t, unmarshalledResponse.Error)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)

	uploadId := unmarshalledResponse.UploadId
	assert.NotEmpty(t, uploadId)
	if uploadId == "" {
		return
	}
	//init upload socket
	uploadSocket := firstApp.establishFileUploadSocket(uploadId)
	ctrlChannel := make(chan string)
	progressChannel := make(chan int)

	//TODO: will this work?
	go uploadSocket.WriteWholeFile(testFile, ctrlChannel, progressChannel)

	go func() {
		progress := <-progressChannel
		if progress == 50 {
			_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileRequest.FileName))
			assert.False(t, os.IsNotExist(err))
			ctrlChannel <- "CANCEL"
		}
	}()

	//check file is there
	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileRequest.FileName))
	assert.True(t, os.IsNotExist(err))
}

func TestFileDelete(t *testing.T) {

	fileName := putGeneralized(t, false)

	//check file is there
	_, err := os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileName))
	assert.False(t, os.IsNotExist(err))

	// send delete
	fileDelRequest := sharedInterfaces.DeleteObjectRequest{
		FileName: fileName,
	}

	unmarshalledDelete, err := firstApp.servConn.FsDelete(fileDelRequest)
	assert.Empty(t, unmarshalledDelete.Error)
	assert.NoError(t, err)
	assert.True(t, unmarshalledDelete.DidSucceed)

	//check file is gone

	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileName))
	assert.True(t, os.IsNotExist(err))
}

func TestDownload(t *testing.T) {

	downloadTarget := putGeneralized(t, false)

	downloadFile := createDownloadFile(t, zcrypto.GenerateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := sharedInterfaces.GetObjectRequest{
		FileName: downloadTarget,
	}

	unmarshalledResponse, err := firstApp.servConn.FsGet(fileRequest)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Error)

	downloadId := unmarshalledResponse.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := firstApp.establishFileDownloadSocket(downloadId)
	msgJson, err := downloadSocket.ReadWholeFile(downloadFile, nil, nil)
	assert.NoError(t, err)
	assert.True(t, msgJson.DidSucceed)
	assert.Equal(t, fileRequest.FileName, msgJson.FileName)

	//hashes of input and output match
	originalSha, err := zcrypto.Sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)

	uploadSha, err := zcrypto.Sha256File(path.Join(firstApp.path, "zome/data", firstApp.originBase, downloadTarget))
	assert.NoError(t, err)

	dlSha, err := zcrypto.Sha256File(downloadFile.Name())
	assert.NoError(t, err)

	assert.Equal(t, originalSha, uploadSha)
	assert.Equal(t, originalSha, dlSha)
}

func TestDownloadCrypt(t *testing.T) {

	downloadTarget := putGeneralized(t, true)

	downloadFile := createDownloadFile(t, zcrypto.GenerateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := sharedInterfaces.GetObjectRequest{
		FileName:   downloadTarget,
		Encryption: true,
	}

	unmarshalledResponse, err := firstApp.servConn.FsGet(fileRequest)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Error)

	downloadId := unmarshalledResponse.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := firstApp.establishFileDownloadSocket(downloadId)

	err = downloadSocket.InitDownload(downloadId)
	assert.NoError(t, err)

	msgJson, err := downloadSocket.ReadWholeFile(downloadFile, nil, nil)
	assert.True(t, msgJson.DidSucceed)
	assert.Equal(t, fileRequest.FileName, msgJson.FileName)

	//hashes of input and output match
	originalSha, err := zcrypto.Sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)

	uploadSha, err := zcrypto.Sha256File(path.Join(firstApp.path, "zome/data", firstApp.originBase, downloadTarget))
	assert.NoError(t, err)

	downloadSha, err := zcrypto.Sha256File(downloadFile.Name())
	assert.NoError(t, err)

	assert.NotEqual(t, originalSha, uploadSha)
	assert.Equal(t, originalSha, downloadSha)
}

func TestDownloadCancel(t *testing.T) {

	downloadTarget := putGeneralized(t, false)

	downloadFile := createDownloadFile(t, zcrypto.GenerateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := sharedInterfaces.GetObjectRequest{
		FileName: downloadTarget,
	}

	unmarshalledResponse, err := firstApp.servConn.FsGet(fileRequest)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Error)

	downloadId := unmarshalledResponse.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := firstApp.establishFileDownloadSocket(downloadId)

	err = downloadSocket.InitDownload(downloadId)
	assert.NoError(t, err)

	msgJson, err := downloadSocket.ReadWholeFile(downloadFile, nil, nil)
	assert.True(t, msgJson.DidSucceed)
	assert.Equal(t, fileRequest.FileName, msgJson.FileName)

	//hashes of input and output do not match
	newSha, err := zcrypto.Sha256File(downloadFile.Name())
	assert.NoError(t, err)

	ogSha, err := zcrypto.Sha256File(path.Join(firstApp.path, "zome/data", firstApp.originBase, downloadTarget))
	assert.NoError(t, err)

	assert.NotEqual(t, ogSha, newSha)
}

func TestDownloadFromPoint(t *testing.T) {

	downloadTarget := putGeneralized(t, false)

	downloadFile := createDownloadFile(t, zcrypto.GenerateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := sharedInterfaces.GetObjectRequest{
		FileName:     downloadTarget,
		ContinueFrom: 4096,
	}

	unmarshalledResponse, err := firstApp.servConn.FsGet(fileRequest)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Error)

	downloadId := unmarshalledResponse.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := firstApp.establishFileDownloadSocket(downloadId)

	err = downloadSocket.InitDownload(downloadId)
	assert.NoError(t, err)

	msgJson, err := downloadSocket.ReadWholeFile(downloadFile, nil, nil)
	assert.True(t, msgJson.DidSucceed)
	assert.Equal(t, fileRequest.FileName, msgJson.FileName)

	//hashes of input and output match
	newSha, err := zcrypto.Sha256File(downloadFile.Name())
	assert.NoError(t, err)

	ogSha, err := zcrypto.Sha256File(path.Join(firstApp.path, "zome/data", firstApp.originBase, downloadTarget))
	assert.NoError(t, err)

	assert.NotEqual(t, ogSha, newSha)
}

func TestSetGetFACL(t *testing.T) {

	// now check false
	unmarshalledResponse, err := firstApp.servConn.FsSetGlobalWrite(sharedInterfaces.SetGlobalFACLRequest{Value: false})

	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Error)
	// now check if the value was set
	unmarshalledGW, err := firstApp.servConn.FsGetGlobalWrite()
	assert.NoError(t, err)
	assert.False(t, unmarshalledGW.GlobalFsAccess)
	assert.Empty(t, unmarshalledGW.Error)

	// now check true
	unmarshalledResponse, err = firstApp.servConn.FsSetGlobalWrite(sharedInterfaces.SetGlobalFACLRequest{Value: true})
	assert.Empty(t, unmarshalledResponse.Error)
	assert.True(t, unmarshalledResponse.DidSucceed)
	assert.NoError(t, err)

	// now check if the value was set
	unmarshalledGW, err = firstApp.servConn.FsGetGlobalWrite()
	assert.NoError(t, err)
	assert.Empty(t, unmarshalledGW.Error)

	assert.True(t, unmarshalledGW.GlobalFsAccess)
}

func TestGetDirectoryListing(t *testing.T) {

	fileName := putGeneralized(t, false)
	// get path of folder
	filePath := path.Dir(fileName)
	// get directory listing
	unmarshalledResponse, err := firstApp.servConn.FsGetListing(sharedInterfaces.GetDirectoryListingRequest{Directory: filePath})
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Error)

	assert.Contains(t, unmarshalledResponse.Files, fileName)
	assert.NotEmpty(t, unmarshalledResponse.Files[fileName])
}

func TestRemoveObjectOrigin(t *testing.T) {

	fileName := putGeneralized(t, false)
	// get path of folder
	filePath := path.Dir(fileName)
	// delete directory listing
	unmarshalledResponse, err := firstApp.servConn.FsRemoveOrigin(sharedInterfaces.RemoveObjectOriginRequest{Directory: filePath})
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Error)
	//check directory is not in data folder
	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, filePath))
	assert.True(t, os.IsNotExist(err))
}
