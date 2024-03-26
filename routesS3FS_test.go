package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func (tc *testContext) establishFileUploadSocket(uploadId string) *websocket.Conn {
	header := http.Header{}
	header.Add("Origin", "https://"+tc.originBase)
	controlSocket, _, err := websocket.DefaultDialer.Dial("ws://"+tc.requestDomain+"/v1/upload/"+uploadId, header)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return controlSocket
}

func (tc *testContext) establishFileDownloadSocket(uploadId string) *websocket.Conn {
	header := http.Header{}
	header.Add("Origin", "https://"+tc.originBase)
	controlSocket, _, err := websocket.DefaultDialer.Dial("ws://"+tc.requestDomain+"/v1/download/"+uploadId, header)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return controlSocket
}

func (tc *testContext) getTestFile() *os.File {
	file, err := os.OpenFile(path.Join(testPathBase, "./testfile.jpg"), os.O_RDONLY, 0755)
	if err != nil {
		panic(err)
	}
	return file
}

func putGeneralized(t *testing.T, controlSocket *websocket.Conn, encryption bool) string {
	rawMeta := map[string]string{
		"test1":   generateRandomKey(),
		"testTwo": generateRandomKey(),
		"from":    "zipdisk",
	}

	testFile := firstApp.getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := generateRandomKey()
	fileRequest := struct {
		FileName     string            `json:"filename"`
		FileSize     int64             `json:"filesize"`
		Tagging      map[string]string `json:"tagging"`
		OverridePath string            `json:"overridePath"`
		ACL          string            `json:"acl"`  // acl of the metadata
		FACL         string            `json:"facl"` // acl of the file within metadata
		Encryption   bool              `json:"encryption"`
	}{
		FileName:     path.Join(randPath, path.Base(testFile.Name())),
		FileSize:     fileSize,
		Tagging:      rawMeta, //this should probly just be nested json
		OverridePath: "",
		ACL:          "11",
		FACL:         "11",
		Encryption:   encryption,
	}

	err = controlSocket.WriteJSON(Request{
		Action: "fs-put",
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
	assert.Empty(t, unmarshalledResponse.Status.Error)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)

	uploadId := unmarshalledResponse.Status.UploadId
	assert.NotEmpty(t, uploadId)
	if uploadId == "" {
		return ""
	}
	// init upload socket
	uploadSocket := firstApp.establishFileUploadSocket(uploadId)

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

		if i == numChunks {
			_, msg, err := uploadSocket.ReadMessage()
			assert.NoError(t, err)
			assert.NotEmpty(t, msg)
			//write 0 to finish upload
			err = uploadSocket.WriteMessage(websocket.BinaryMessage, []byte("EOF"))
			assert.NoError(t, err)

			_, msg, err = uploadSocket.ReadMessage()
			assert.NoError(t, err)
			assert.NotEmpty(t, msg)

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
		} else {
			_, msg, err := uploadSocket.ReadMessage()
			assert.NoError(t, err)
			assert.NotEmpty(t, msg)
		}
		// assert.Equal(t, []byte("{\"pct\":"+fmt.Sprint((((i*chunkSize)*100)/fileRequest.FileSize)+1)+"}"), msg)
	}

	// hashes of input and output match
	ogSha, err := sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)

	newPath := path.Join(firstApp.path, "zome/data", firstApp.originBase, fileRequest.FileName)
	newSha, err := sha256File(newPath)
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
	// Test case 1: Valid request
	controlSocket := firstApp.establishControlSocket()
	putGeneralized(t, controlSocket, false)
}

func TestFileSeek(t *testing.T) {
	// Test case 1: Valid request
	rawMeta := map[string]string{
		"test1":   generateRandomKey(),
		"testTwo": generateRandomKey(),
		"from":    "zipdisk",
	}

	testFile := firstApp.getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := generateRandomKey()
	fileRequest := struct {
		FileName string            `json:"filename"`
		FileSize int64             `json:"filesize"`
		Tagging  map[string]string `json:"tagging"`
		ACL      string            `json:"acl"`  // acl of the metadata
		FACL     string            `json:"facl"` // acl of the file within metadata
	}{
		FileName: path.Join(randPath, path.Base(testFile.Name())),
		FileSize: fileSize,
		Tagging:  rawMeta,
		ACL:      "11",
		FACL:     "11",
	}

	controlSocket := firstApp.establishControlSocket()

	err = controlSocket.WriteJSON(Request{
		Action: "fs-put",
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
	assert.Empty(t, unmarshalledResponse.Status.Error)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)

	uploadId := unmarshalledResponse.Status.UploadId
	assert.NotEmpty(t, uploadId)
	if uploadId == "" {
		return
	}
	//init upload socket
	uploadSocket := firstApp.establishFileUploadSocket(uploadId)

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

		if strings.Contains(string(msg), "100") {
			//write eof to finish upload
			err = uploadSocket.WriteMessage(websocket.BinaryMessage, []byte("EOF"))
			assert.NoError(t, err)
			_, msg, err := uploadSocket.ReadMessage()
			assert.NoError(t, err)
			assert.NotEmpty(t, msg)

			msgJson := struct {
				Code   int `json:"code"`
				Status struct {
					DidSucceed bool   `json:"didSucceed"`
					Hash       string `json:"hash"`
					FileName   string `json:"fileName"`
					BytesRead  int64  `json:"bytesRead"`
					Error      string `json:"error"`
				} `json:"status"`
			}{}
			err = json.Unmarshal(msg, &msgJson)
			assert.NoError(t, err)
			assert.True(t, msgJson.Status.DidSucceed)
			assert.Empty(t, msgJson.Status.Error)

			assert.Equal(t, fileRequest.FileName, msgJson.Status.FileName)
			assert.Equal(t, fileRequest.FileSize, int64(111853)) // msgJson.Status.BytesRead
		}

		// assert.Equal(t, []byte("{\"pct\":"+fmt.Sprint((((i*chunkSize)*100)/fileRequest.FileSize)+1)+"}"), msg)
	}

	//check file is there
	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileRequest.FileName))
	assert.False(t, os.IsNotExist(err))
	//verify that it's only (second) half populated?
}

func TestFileCancel(t *testing.T) {
	// Test case 1: Valid request
	rawMeta := map[string]string{
		"test1":   generateRandomKey(),
		"testTwo": generateRandomKey(),
		"from":    "zipdisk",
	}

	testFile := firstApp.getTestFile()
	defer testFile.Close()

	fi, err := testFile.Stat()
	assert.NoError(t, err)

	fileSize := fi.Size()
	randPath := generateRandomKey()
	fileRequest := struct {
		FileName string            `json:"filename"`
		FileSize int64             `json:"filesize"`
		Tagging  map[string]string `json:"tagging"`
		ACL      string            `json:"acl"`  // acl of the metadata
		FACL     string            `json:"facl"` // acl of the file within metadata
	}{
		FileName: path.Join(randPath, path.Base(testFile.Name())),
		FileSize: fileSize,
		Tagging:  rawMeta,
		ACL:      "11",
		FACL:     "11",
	}

	controlSocket := firstApp.establishControlSocket()

	err = controlSocket.WriteJSON(Request{
		Action: "fs-put",
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
	uploadSocket := firstApp.establishFileUploadSocket(uploadId)

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

	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileRequest.FileName))
	assert.False(t, os.IsNotExist(err))

	err = uploadSocket.WriteMessage(websocket.TextMessage, []byte("CANCEL"))
	assert.NoError(t, err)
	_, msg, err := uploadSocket.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, []byte("{\"code\":200,\"status\":\"Upload canceled\"}"), msg)

	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileRequest.FileName))
	assert.True(t, os.IsNotExist(err))
}

func TestFileDelete(t *testing.T) {
	controlSocket := firstApp.establishControlSocket()

	fileName := putGeneralized(t, controlSocket, false)

	//check file is there

	_, err := os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileName))
	assert.False(t, os.IsNotExist(err))

	// send delete
	fileDelRequest := struct {
		FileName string `json:"fileName"`
	}{
		FileName: fileName,
	}

	err = controlSocket.WriteJSON(Request{
		Action: "fs-delete",
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
	assert.Empty(t, unmarshalledDelete.Status.Error)
	assert.NoError(t, err)
	assert.True(t, unmarshalledDelete.Status.DidSucceed)

	//check file is gone

	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, fileName))
	assert.True(t, os.IsNotExist(err))
}

func TestDownload(t *testing.T) {
	controlSocket := firstApp.establishControlSocket()

	downloadTarget := putGeneralized(t, controlSocket, false)

	downloadFile := createDownloadFile(t, generateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := struct {
		FileName string `json:"fileName"`
	}{
		FileName: downloadTarget,
	}

	err := controlSocket.WriteJSON(Request{
		Action: "fs-get",
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
	assert.Empty(t, unmarshalledResponse.Status.Error)

	downloadId := unmarshalledResponse.Status.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := firstApp.establishFileDownloadSocket(downloadId)

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
	originalSha, err := sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)

	uploadSha, err := sha256File(path.Join(firstApp.path, "zome/data", firstApp.originBase, downloadTarget))
	assert.NoError(t, err)

	dlSha, err := sha256File(downloadFile.Name())
	assert.NoError(t, err)

	assert.Equal(t, originalSha, uploadSha)
	assert.Equal(t, originalSha, dlSha)
}

func TestDownloadCrypt(t *testing.T) {
	controlSocket := firstApp.establishControlSocket()

	downloadTarget := putGeneralized(t, controlSocket, true)

	downloadFile := createDownloadFile(t, generateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := struct {
		FileName   string `json:"fileName"`
		Encryption bool   `json:"encryption"`
	}{
		FileName:   downloadTarget,
		Encryption: true,
	}

	err := controlSocket.WriteJSON(Request{
		Action: "fs-get",
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
	assert.Empty(t, unmarshalledResponse.Status.Error)

	downloadId := unmarshalledResponse.Status.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := firstApp.establishFileDownloadSocket(downloadId)

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
	originalSha, err := sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)

	uploadSha, err := sha256File(path.Join(firstApp.path, "zome/data", firstApp.originBase, downloadTarget))
	assert.NoError(t, err)

	downloadSha, err := sha256File(downloadFile.Name())
	assert.NoError(t, err)

	assert.NotEqual(t, originalSha, uploadSha)
	assert.Equal(t, originalSha, downloadSha)
}

func TestDownloadCancel(t *testing.T) {
	controlSocket := firstApp.establishControlSocket()

	downloadTarget := putGeneralized(t, controlSocket, false)

	downloadFile := createDownloadFile(t, generateRandomKey()+"-dlfile.jpg")
	defer downloadFile.Close()

	// make download folder

	fileRequest := struct {
		FileName string `json:"fileName"`
	}{
		FileName: downloadTarget,
	}

	err := controlSocket.WriteJSON(Request{
		Action: "fs-get",
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
	assert.Empty(t, unmarshalledResponse.Status.Error)

	downloadId := unmarshalledResponse.Status.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := firstApp.establishFileDownloadSocket(downloadId)

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

	ogSha, err := sha256File(path.Join(firstApp.path, "zome/data", firstApp.originBase, downloadTarget))
	assert.NoError(t, err)

	assert.NotEqual(t, ogSha, newSha)
}

func TestDownloadFromPoint(t *testing.T) {
	controlSocket := firstApp.establishControlSocket()

	downloadTarget := putGeneralized(t, controlSocket, false)

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
		Action: "fs-get",
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
	assert.Empty(t, unmarshalledResponse.Status.Error)

	downloadId := unmarshalledResponse.Status.DownloadId
	assert.NotEmpty(t, downloadId)
	if downloadId == "" {
		return
	}
	//init download socket
	downloadSocket := firstApp.establishFileDownloadSocket(downloadId)

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

	ogSha, err := sha256File(path.Join(firstApp.path, "zome/data", firstApp.originBase, downloadTarget))
	assert.NoError(t, err)

	assert.NotEqual(t, ogSha, newSha)
}

func TestSetGetFACL(t *testing.T) {
	// Test case 1: Valid request

	controlSocket := firstApp.establishControlSocket()

	type successStruct struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			Error      string `json:"error"`
		} `json:"status"`
	}

	type gwStruct struct {
		Code   int `json:"code"`
		Status struct {
			GlobalFsAccess bool   `json:"globalFsAccess"`
			Error          string `json:"error"`
		} `json:"status"`
	}

	// now check false
	err := controlSocket.WriteJSON(Request{
		Action: "fs-setGlobalWrite",
		Data: struct {
			Value bool `json:"value"`
		}{Value: false},
	})
	assert.NoError(t, err)

	unmarshalledResponse := successStruct{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Status.Error)
	// now check if the value was set
	err = controlSocket.WriteJSON(Request{
		Action: "fs-getGlobalWrite",
	})
	assert.NoError(t, err)
	unmarshalledGW := gwStruct{}
	err = controlSocket.ReadJSON(&unmarshalledGW)
	assert.NoError(t, err)
	assert.False(t, unmarshalledGW.Status.GlobalFsAccess)
	assert.Empty(t, unmarshalledGW.Status.Error)

	err = controlSocket.WriteJSON(Request{
		Action: "fs-setGlobalWrite",
		Data: struct {
			Value bool   `json:"value"`
			Error string `json:"error"`
		}{Value: true},
	})
	assert.NoError(t, err)

	unmarshalledResponse = successStruct{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.Empty(t, unmarshalledResponse.Status.Error)
	assert.NoError(t, err)

	assert.True(t, unmarshalledResponse.Status.DidSucceed)
	// now check if the value was set
	err = controlSocket.WriteJSON(Request{
		Action: "fs-getGlobalWrite",
	})
	assert.NoError(t, err)
	unmarshalledGW = gwStruct{}
	err = controlSocket.ReadJSON(&unmarshalledGW)
	assert.NoError(t, err)
	assert.Empty(t, unmarshalledGW.Status.Error)

	assert.True(t, unmarshalledGW.Status.GlobalFsAccess)

}

func TestGetDirectoryListing(t *testing.T) {
	controlSocket := firstApp.establishControlSocket()
	fileName := putGeneralized(t, controlSocket, false)
	// get path of folder
	filePath := path.Dir(fileName)
	// get directory listing
	err := controlSocket.WriteJSON(Request{
		Action: "fs-getListing",
		Data: struct {
			Directory string `json:"directory"`
		}{Directory: filePath},
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool              `json:"didSucceed"`
			Files      map[string]string `json:"files"`
			Error      string            `json:"error"`
		} `json:"status"`
	}{}
	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Status.Error)

	assert.Contains(t, unmarshalledResponse.Status.Files, fileName)
	assert.NotEmpty(t, unmarshalledResponse.Status.Files[fileName])

}

func TestRemoveObjectOrigin(t *testing.T) {
	controlSocket := firstApp.establishControlSocket()
	fileName := putGeneralized(t, controlSocket, false)
	// get path of folder
	filePath := path.Dir(fileName)
	// delete directory listing
	err := controlSocket.WriteJSON(Request{
		Action: "fs-removeOrigin",
		Data: struct {
			Directory string `json:"directory"`
		}{Directory: filePath},
	})
	assert.NoError(t, err)
	unmarshalledResponse := struct {
		Code   int `json:"code"`
		Status struct {
			DidSucceed bool   `json:"didSucceed"`
			Error      string `json:"error"`
		} `json:"status"`
	}{}

	err = controlSocket.ReadJSON(&unmarshalledResponse)
	assert.NoError(t, err)
	assert.True(t, unmarshalledResponse.Status.DidSucceed)
	assert.Empty(t, unmarshalledResponse.Status.Error)
	//check directory is not in data folder
	_, err = os.Stat(path.Join(firstApp.path, "zome/data", firstApp.originBase, filePath))
	assert.True(t, os.IsNotExist(err))

}
