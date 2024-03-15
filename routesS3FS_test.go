package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

const uploadEndpoint = "ws://localhost:5253/v1/upload/"
const downloadEndpoint = "ws://localhost:5253/v1/download/"

func establishFileUploadSocket(uploadId string) *websocket.Conn {
	header := http.Header{}
	header.Add("Origin", originVar)
	controlSocket, _, err := websocket.DefaultDialer.Dial(uploadEndpoint+uploadId, header)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return controlSocket
}

func establishFileDownloadSocket(uploadId string) *websocket.Conn {
	header := http.Header{}
	header.Add("Origin", originVar)
	controlSocket, _, err := websocket.DefaultDialer.Dial(downloadEndpoint+uploadId, header)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return controlSocket
}

func getTestFile() *os.File {
	file, err := os.OpenFile("./testPath/testfile.jpg", os.O_RDONLY, 0755)
	if err != nil {
		panic(err)
	}
	return file
}

func putGeneralized(t *testing.T, controlSocket *websocket.Conn) string {
	urlQuerystring :=
		"test1=" + generateRandomKey() +
			"&testTwo=" + generateRandomKey() +
			"&from=zipdisk"

	testFile := getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := generateRandomKey()
	fileRequest := struct {
		FileName string `json:"filename"`
		FileSize int64  `json:"filesize"`
		Tagging  string `json:"tagging"`
	}{

		FileName: path.Join(randPath, path.Base(testFile.Name())),
		FileSize: fileSize,
		Tagging:  urlQuerystring,
	}

	err = controlSocket.WriteJSON(Request{
		Action: "fs-putObject",
		Data:   fileRequest,
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			UploadId   string `json:"uploadId"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)

	uploadId := unmarshalledResponse.Status.UploadId
	assert.NotEmpty(t, uploadId)
	if uploadId == "" {
		return ""
	}
	// init upload socket
	uploadSocket := establishFileUploadSocket(uploadId)

	// iterate over chunks of testfile and send them to the server
	chunkSize := int64(1024)
	numChunks := fileRequest.FileSize / chunkSize
	for i := int64(0); i <= numChunks; i++ {
		cpos := i * chunkSize
		chunk := make([]byte, chunkSize)
		if i*chunkSize+chunkSize > fileRequest.FileSize {
			chunk = make([]byte, fileRequest.FileSize-i*chunkSize)
		}

		_, err = testFile.ReadAt(chunk, cpos)

		assert.NoError(t, err)
		err = uploadSocket.WriteMessage(websocket.BinaryMessage, chunk)
		assert.NoError(t, err)
		_, msg, err := uploadSocket.ReadMessage()
		assert.NoError(t, err)
		assert.NotEmpty(t, msg)
		if i == numChunks {
			msgJson := struct {
				Code   int `json:"code"`
				Status struct {
					DidSucceed bool   `json:"didSucceed"`
					Hash       string `json:"hash"`
					FileName   string `json:"fileName"`
					BytesRead  int64  `json:"bytesRead"`
				} `json:"status"`
			}{}
			err = json.Unmarshal(msg, &msgJson)
			assert.NoError(t, err)
			assert.NoError(t, err)
			assert.True(t, msgJson.Status.DidSucceed)
			assert.Equal(t, fileRequest.FileName, msgJson.Status.FileName)
			assert.Equal(t, fileRequest.FileSize, msgJson.Status.BytesRead)
		}
		// assert.Equal(t, []byte("{\"pct\":"+fmt.Sprint((((i*chunkSize)*100)/fileRequest.FileSize)+1)+"}"), msg)
	}

	// hashes of input and output match
	ogSha, err := sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)

	baseurl, err := url.Parse(originVar)
	assert.NoError(t, err)
	originSeg := baseurl.Hostname()
	newPath := path.Join("testPath", "zome", "data", originSeg, fileRequest.FileName)
	newSha, err := sha256File(newPath)
	assert.NoError(t, err)

	assert.Equal(t, ogSha, newSha)
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
	// Test case 1: Valid request
	controlSocket := establishControlSocket()
	putGeneralized(t, controlSocket)
}

func TestFileSeek(t *testing.T) {
	// Test case 1: Valid request
	urlQuerystring :=
		"test1=" + generateRandomKey() +
			"&testTwo=" + generateRandomKey() +
			"&from=zipdisk"

	testFile := getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := generateRandomKey()
	fileRequest := struct {
		FileName string `json:"filename"`
		FileSize int64  `json:"filesize"`
		Tagging  string `json:"tagging"`
	}{
		FileName: path.Join(randPath, path.Base(testFile.Name())),
		FileSize: fileSize,
		Tagging:  urlQuerystring,
	}

	controlSocket := establishControlSocket()

	err = controlSocket.WriteJSON(Request{
		Action: "fs-putObject",
		Data:   fileRequest,
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			UploadId   string `json:"uploadId"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)

	uploadId := unmarshalledResponse.Status.UploadId
	assert.NotEmpty(t, uploadId)
	if uploadId == "" {
		return
	}
	//init upload socket
	uploadSocket := establishFileUploadSocket(uploadId)

	//iterate over chunks of testfile and send them to the server
	chunkSize := int64(1024)
	halfChunks := (fileRequest.FileSize / 2) / chunkSize
	skipString := strconv.FormatInt(fileRequest.FileSize/2, 10)
	err = uploadSocket.WriteMessage(websocket.TextMessage, []byte("SKIPTO"+skipString))
	assert.NoError(t, err)
	_, msg, err := uploadSocket.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, []byte("{\"code\":200,\"status\":\"Skipping to "+skipString+"\"}"), msg)

	for i := int64(0); i <= halfChunks; i++ { //iter8 over the second halfa the chunx
		cpos := i * chunkSize
		chunk := make([]byte, chunkSize)
		if i*chunkSize+chunkSize > fileRequest.FileSize {
			chunk = make([]byte, fileRequest.FileSize-i*chunkSize)
		}

		_, err = testFile.ReadAt(chunk, cpos)

		assert.NoError(t, err)
		err = uploadSocket.WriteMessage(websocket.BinaryMessage, chunk)
		assert.NoError(t, err)
		_, msg, err := uploadSocket.ReadMessage()
		assert.NoError(t, err)
		assert.NotEmpty(t, msg)

		if i == halfChunks {
			msgJson := struct {
				Code   int `json:"code"`
				Status struct {
					DidSucceed bool   `json:"didSucceed"`
					Hash       string `json:"hash"`
					FileName   string `json:"fileName"`
					BytesRead  int64  `json:"bytesRead"`
				} `json:"status"`
			}{}
			err = json.Unmarshal(msg, &msgJson)
			assert.NoError(t, err)
			assert.True(t, msgJson.Status.DidSucceed)
			assert.Equal(t, fileRequest.FileName, msgJson.Status.FileName)
			// assert.Equal(t, fileRequest.FileSize, statusUnmarshalled.BytesRead)//TODO: doesn't match
		}

		// assert.Equal(t, []byte("{\"pct\":"+fmt.Sprint((((i*chunkSize)*100)/fileRequest.FileSize)+1)+"}"), msg)
	}

	//check file is there
	baseurl, err := url.Parse(originVar)
	assert.NoError(t, err)
	originSeg := baseurl.Hostname()
	_, err = os.Stat(path.Join("testPath", "zome", "data", originSeg, fileRequest.FileName))
	assert.False(t, os.IsNotExist(err))
	//verify that it's only (second) half populated?
}

func TestFileCancel(t *testing.T) {
	// Test case 1: Valid request
	urlQuerystring :=
		"test1=" + generateRandomKey() +
			"&testTwo=" + generateRandomKey() +
			"&from=zipdisk"

	testFile := getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := generateRandomKey()
	fileRequest := struct {
		FileName string `json:"filename"`
		FileSize int64  `json:"filesize"`
		Tagging  string `json:"tagging"`
	}{
		FileName: path.Join(randPath, path.Base(testFile.Name())),
		FileSize: fileSize,
		Tagging:  urlQuerystring,
	}

	controlSocket := establishControlSocket()

	err = controlSocket.WriteJSON(Request{
		Action: "fs-putObject",
		Data:   fileRequest,
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			UploadId   string `json:"uploadId"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)

	uploadId := unmarshalledResponse.Status.UploadId
	assert.NotEmpty(t, uploadId)
	if uploadId == "" {
		return
	}
	//init upload socket
	uploadSocket := establishFileUploadSocket(uploadId)

	//iterate over chunks of testfile and send them to the server
	chunkSize := int64(1024)
	numChunks := fileRequest.FileSize / chunkSize

	for i := int64(0); i <= numChunks/2; i++ { //only iterate over first halfa the chunx
		cpos := i * chunkSize
		chunk := make([]byte, chunkSize)
		if i*chunkSize+chunkSize > fileRequest.FileSize {
			chunk = make([]byte, fileRequest.FileSize-i*chunkSize)
		}
		_, err = testFile.ReadAt(chunk, cpos)

		assert.NoError(t, err)
		err = uploadSocket.WriteMessage(websocket.BinaryMessage, chunk)
		assert.NoError(t, err)
		_, msg, err := uploadSocket.ReadMessage()
		assert.NoError(t, err)
		assert.NotEmpty(t, msg)
		// assert.Equal(t, []byte("{\"pct\":"+fmt.Sprint((((i*chunkSize)*100)/fileRequest.FileSize)+1)+"}"), msg)
	}

	//check file is there
	baseurl, err := url.Parse(originVar)
	assert.NoError(t, err)
	originSeg := baseurl.Hostname()
	_, err = os.Stat(path.Join("testPath", "zome", "data", originSeg, fileRequest.FileName))
	assert.False(t, os.IsNotExist(err))

	err = uploadSocket.WriteMessage(websocket.TextMessage, []byte("CANCEL"))
	assert.NoError(t, err)
	_, msg, err := uploadSocket.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, []byte("{\"code\":200,\"status\":\"Upload canceled\"}"), msg)

	_, err = os.Stat(path.Join("testPath", "zome", "data", originSeg, fileRequest.FileName))
	assert.True(t, os.IsNotExist(err))

}

func TestFileDelete(t *testing.T) {
	controlSocket := establishControlSocket()

	fileName := putGeneralized(t, controlSocket)

	//check file is there
	baseurl, err := url.Parse(originVar)
	assert.NoError(t, err)
	originSeg := baseurl.Hostname()
	_, err = os.Stat(path.Join("testPath", "zome", "data", originSeg, fileName))
	assert.False(t, os.IsNotExist(err))

	// send delete
	fileDelRequest := struct {
		FileName string `json:"fileName"`
	}{
		FileName: fileName,
	}

	err = controlSocket.WriteJSON(Request{
		Action: "fs-deleteObject",
		Data:   fileDelRequest,
	})
	assert.NoError(t, err)
	unmarshalledDelete := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			UploadId   string `json:"uploadId"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledDelete)
	assert.NoError(t, err)
	assert.True(t, unmarshalledDelete.Status.DidSucceed)

	//check file is gone

	_, err = os.Stat(path.Join("testPath", "zome", "data", originSeg, fileName))
	assert.True(t, os.IsNotExist(err))
}

//TODO: complete download(and its cancel/skipto)

func TestDownload(t *testing.T) {
	controlSocket := establishControlSocket()

	downloadTarget := putGeneralized(t, controlSocket)

	downloadFile := createDownloadFile(t, generateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := struct {
		FileName string `json:"fileName"`
	}{
		FileName: downloadTarget,
	}

	err := controlSocket.WriteJSON(Request{
		Action: "fs-getObject",
		Data:   fileRequest,
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			DownloadId string `json:"downloadId"`
			MetaData   string `json:"metadata"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)

	downloadId := unmarshalledResponse.Status.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := establishFileDownloadSocket(downloadId)

	//get expected size
	sizeStruct := struct {
		Code   int `json:"code"`
		Status struct {
			Size int64 `json:"size"`
		} `json:"status"`
	}{}
	err = downloadSocket.ReadJSON(&sizeStruct)
	assert.NoError(t, err)
	assert.NotEmpty(t, sizeStruct.Status.Size)

	//pull chunks from download socket and feed them into testfile
	for {
		// err = downloadSocket.WriteMessage(websocket.BinaryMessage, []byte(""))
		// assert.NoError(t, err)
		mType, chunk, err := downloadSocket.ReadMessage()
		assert.NoError(t, err)

		//check if we've written more than the expected size
		if int64(len(chunk)) > sizeStruct.Status.Size {
			assert.Fail(t, "Downloaded more than expected size")
		}

		fi, err := downloadFile.Stat()
		assert.NoError(t, err)
		fiSize := fi.Size()

		//check if we've written the expected size
		if fiSize == sizeStruct.Status.Size {
			break
		}

		if mType == websocket.TextMessage {
			msgJson := struct {
				Code   int `json:"code"`
				Status struct {
					DidSucceed bool   `json:"didSucceed"`
					FileName   string `json:"fileName"`
				} `json:"status"`
			}{}
			err = json.Unmarshal(chunk, &msgJson)
			assert.NoError(t, err)
			assert.True(t, msgJson.Status.DidSucceed)
			assert.Equal(t, fileRequest.FileName, msgJson.Status.FileName)
			break
		}

		_, err = downloadFile.Write(chunk)
		assert.NoError(t, err)
	}

	//hashes of input and output match
	newSha, err := sha256File(downloadFile.Name())
	assert.NoError(t, err)

	baseurl, err := url.Parse(originVar)
	assert.NoError(t, err)
	originSeg := baseurl.Hostname()
	ogSha, err := sha256File(path.Join("testPath", "zome", "data", originSeg, downloadTarget))
	assert.NoError(t, err)

	assert.Equal(t, ogSha, newSha)
}

func TestDownloadCancel(t *testing.T) {
	controlSocket := establishControlSocket()

	downloadTarget := putGeneralized(t, controlSocket)

	downloadFile := createDownloadFile(t, generateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := struct {
		FileName string `json:"fileName"`
	}{
		FileName: downloadTarget,
	}

	err := controlSocket.WriteJSON(Request{
		Action: "fs-getObject",
		Data:   fileRequest,
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			DownloadId string `json:"downloadId"`
			MetaData   string `json:"metadata"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)

	downloadId := unmarshalledResponse.Status.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := establishFileDownloadSocket(downloadId)

	//get expected size
	sizeStruct := struct {
		Code   int `json:"code"`
		Status struct {
			Size int64 `json:"size"`
		} `json:"status"`
	}{}
	err = downloadSocket.ReadJSON(&sizeStruct)
	assert.NoError(t, err)
	assert.NotEmpty(t, sizeStruct.Status.Size)

	// numChunks := sizeStruct.Status.Size / 1024
	chunkCount := int64(0)

	//pull chunks from download socket and feed them into testfile
	for {
		mType, chunk, err := downloadSocket.ReadMessage()
		assert.NoError(t, err)

		//check if we've written more than the expected size
		if int64(len(chunk)) > sizeStruct.Status.Size {
			assert.Fail(t, "Downloaded more than expected size")
		}

		fi, err := downloadFile.Stat()
		assert.NoError(t, err)
		fiSize := fi.Size()

		// cancel download halfway through
		if fiSize >= sizeStruct.Status.Size/2 {
			downloadSocket.Close()
			downloadFile.Close()
			break
		}

		//check if we've written the expected size
		if fiSize == sizeStruct.Status.Size {
			downloadSocket.Close()
			downloadFile.Close()
			break
		}

		if mType == websocket.TextMessage {
			msgJson := struct {
				Code   int `json:"code"`
				Status struct {
					DidSucceed bool   `json:"didSucceed"`
					FileName   string `json:"fileName"`
				} `json:"status"`
			}{}
			err = json.Unmarshal(chunk, &msgJson)
			assert.NoError(t, err)
			assert.True(t, msgJson.Status.DidSucceed)
			assert.Equal(t, fileRequest.FileName, msgJson.Status.FileName)
			break
		}

		_, err = downloadFile.Write(chunk)
		chunkCount += 1
		assert.NoError(t, err)
	}

	//hashes of input and output do not match
	newSha, err := sha256File(downloadFile.Name())
	assert.NoError(t, err)

	baseurl, err := url.Parse(originVar)
	assert.NoError(t, err)
	originSeg := baseurl.Hostname()
	ogSha, err := sha256File(path.Join("testPath", "zome", "data", originSeg, downloadTarget))
	assert.NoError(t, err)

	assert.NotEqual(t, ogSha, newSha)
}

func TestDownloadFromPoint(t *testing.T) {
	controlSocket := establishControlSocket()

	downloadTarget := putGeneralized(t, controlSocket)

	downloadFile := createDownloadFile(t, generateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := struct {
		FileName     string `json:"fileName"`
		ContinueFrom int64  `json:"continueFrom"`
	}{
		FileName:     downloadTarget,
		ContinueFrom: 4096,
	}

	err := controlSocket.WriteJSON(Request{
		Action: "fs-getObject",
		Data:   fileRequest,
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			DownloadId string `json:"downloadId"`
			MetaData   string `json:"metadata"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)

	downloadId := unmarshalledResponse.Status.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := establishFileDownloadSocket(downloadId)

	//get expected size
	sizeStruct := struct {
		Code   int `json:"code"`
		Status struct {
			Size int64 `json:"size"`
		} `json:"status"`
	}{}
	err = downloadSocket.ReadJSON(&sizeStruct)
	assert.NoError(t, err)
	assert.NotEmpty(t, sizeStruct.Status.Size)

	//pull chunks from download socket and feed them into testfile
	for {
		// err = downloadSocket.WriteMessage(websocket.BinaryMessage, []byte(""))
		// assert.NoError(t, err)
		mType, chunk, err := downloadSocket.ReadMessage()
		assert.NoError(t, err)

		//check if we've written more than the expected size
		if int64(len(chunk)) > sizeStruct.Status.Size {
			assert.Fail(t, "Downloaded more than expected size")
		}

		fi, err := downloadFile.Stat()
		assert.NoError(t, err)
		fiSize := fi.Size()

		//check if we've written the expected size
		if fiSize == sizeStruct.Status.Size {
			break
		}

		if mType == websocket.TextMessage {
			msgJson := struct {
				Code   int `json:"code"`
				Status struct {
					DidSucceed bool   `json:"didSucceed"`
					FileName   string `json:"fileName"`
				} `json:"status"`
			}{}
			err = json.Unmarshal(chunk, &msgJson)
			assert.NoError(t, err)
			assert.True(t, msgJson.Status.DidSucceed)
			assert.Equal(t, fileRequest.FileName, msgJson.Status.FileName)
			break
		}

		_, err = downloadFile.Write(chunk)
		assert.NoError(t, err)
	}

	//hashes of input and output match
	newSha, err := sha256File(downloadFile.Name())
	assert.NoError(t, err)

	baseurl, err := url.Parse(originVar)
	assert.NoError(t, err)
	originSeg := baseurl.Hostname()
	ogSha, err := sha256File(path.Join("testPath", "zome", "data", originSeg, downloadTarget))
	assert.NoError(t, err)

	assert.NotEqual(t, ogSha, newSha)
}
