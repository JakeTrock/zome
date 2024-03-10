package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

const uploadEndpoint = "ws://localhost:5253/v1/upload/"

func establishFileSocket(uploadId string) *websocket.Conn {
	header := http.Header{}
	header.Add("Origin", originVar)
	controlSocket, _, err := websocket.DefaultDialer.Dial(uploadEndpoint+uploadId, header)
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

func TestPut(t *testing.T) {
	// Test case 1: Valid request
	urlQuerystring := "test1=" +
		generateRandomKey() +
		"&testTwo=" + generateRandomKey() +
		"&from=zipdisk"

	testFile := getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()

	fileRequest := struct {
		FileName string `json:"filename"`
		FileSize int64  `json:"filesize"`
		Tagging  string `json:"tagging"`
	}{
		FileName: path.Join("dsr300", path.Base(testFile.Name())),
		FileSize: fileSize,
		Tagging:  urlQuerystring,
	}

	marshalledRequest, err := json.Marshal(fileRequest)
	assert.NoError(t, err)

	controlSocket := establishControlSocket()
	defer controlSocket.Close()

	err = controlSocket.WriteJSON(Request{
		Action: "fs-putObject",
		Data:   marshalledRequest,
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		DidSucceed bool   `json:"didSucceed"`
		UploadId   string `json:"uploadId"`
		Error      string `json:"error"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.DidSucceed)

	uploadId := unmarshalledResponse.UploadId
	assert.NotEmpty(t, uploadId)
	//TODO: test uploading testfile to uploadId
	if uploadId == "" {
		return
	}
	//init upload socket
	uploadSocket := establishFileSocket(uploadId)
	defer uploadSocket.Close()

	//iterate over chunks of testfile and send them to the server
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
				Code   int    `json:"code"`
				Status string `json:"status"`
			}{}
			err = json.Unmarshal(msg, &msgJson)
			assert.NoError(t, err)
			statusUnmarshalled := struct {
				DidSucceed bool   `json:"didSucceed"`
				Hash       string `json:"hash"`
				FileName   string `json:"fileName"`
				BytesRead  int64  `json:"bytesRead"`
			}{}
			err = json.Unmarshal([]byte(msgJson.Status), &statusUnmarshalled)
			assert.NoError(t, err)
			assert.True(t, statusUnmarshalled.DidSucceed)
			assert.Equal(t, fileRequest.FileName, statusUnmarshalled.FileName)
			assert.Equal(t, fileRequest.FileSize, statusUnmarshalled.BytesRead)
		}
		// assert.Equal(t, []byte("{\"pct\":"+fmt.Sprint((((i*chunkSize)*100)/fileRequest.FileSize)+1)+"}"), msg)
	}

	//hashes of input and output match
	ogSha, err := sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)

	baseurl, err := url.Parse(originVar)
	assert.NoError(t, err)
	originSeg := baseurl.Hostname()
	newSha, err := sha256File(path.Join("testPath", "zome", "data", originSeg, fileRequest.FileName))
	assert.NoError(t, err)

	assert.Equal(t, ogSha, newSha)
}
