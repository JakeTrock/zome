package libzome

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/libp2p/go-libp2p/core/network"
)

type ZomeUploader struct {
	socket network.Stream
}

const chunkSize = int64(1024)

func (zd *ZomeUploader) InitUpload(uploadId string) error {
	_, err := zd.socket.Write([]byte(uploadId))
	if err != nil {
		return fmt.Errorf("error writing uploadId: %s", err)
	}
	return nil
}

//TODO: atomic appends/writes for logging etc

func (zd *ZomeUploader) WriteWholeFile(fileUpload *os.File, ctrlChannel chan string, progressChannel chan int) (sharedInterfaces.UploadSuccess, error) {
	fi, err := fileUpload.Stat()
	if err != nil {
		return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error getting file stats: %s", err)
	}
	fileSize := fi.Size()

	numChunks := fileSize / chunkSize
	for i := int64(0); i <= numChunks; i++ {
		cpos := i * chunkSize

		// if we get a skip message on the control channel, skip the current chunk
		select {
		case cmsg := <-ctrlChannel:
			if strings.HasPrefix(cmsg, "SKIPTO") {
				_, err = zd.socket.Write([]byte(cmsg))
				if err != nil {
					return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error writing skip message: %s", err)
				}
				continue
			} else if strings.HasPrefix(cmsg, "CANCEL") {
				_, err = zd.socket.Write([]byte("CANCEL"))
				if err != nil {
					return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error writing cancel message: %s", err)
				}
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("upload cancelled")
			}
		default:
		}

		chunk := make([]byte, chunkSize)
		if i*chunkSize+chunkSize > fileSize {
			chunk = make([]byte, fileSize-i*chunkSize)
		}

		_, err = fileUpload.ReadAt(chunk, cpos)
		if err != nil {
			return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error reading file: %s", err)
		}
		_, err = zd.socket.Write(chunk)
		if err != nil {
			return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error writing chunk: %s", err)
		}

		if i == numChunks {
			var msg []byte
			_, err := zd.socket.Read(msg)
			if err != nil {
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error reading upload response: %s", err)
			}
			if string(msg) == "" {
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error reading upload response: empty response")
			}
			progressChannel <- 100 // the message above is the final message(100%)
			//write 0 to finish upload
			_, err = zd.socket.Write([]byte("EOF"))
			if err != nil {
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error writing EOF: %s", err)
			}

			var successMsg []byte
			_, err = zd.socket.Read(successMsg)
			if err != nil {
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error reading upload success message: %s", err)
			}
			if string(successMsg) == "" {
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error reading upload success message: empty response")
			}

			msgJson := struct {
				Code   int                            `json:"code"`
				Status sharedInterfaces.UploadSuccess `json:"status"`
			}{}
			err = json.Unmarshal(successMsg, &msgJson)
			if err != nil {
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error unmarshalling upload success message: %s", err)
			}
			if msgJson.Code != 200 {
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error uploading file: %s", msgJson.Status.Error)
			}
			return msgJson.Status, nil
		} else {
			var progMessage []byte
			_, err := zd.socket.Read(progMessage)
			if err != nil {
				return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error reading upload progress message: %s", err)
			}
			if progressChannel != nil {
				var ulPct sharedInterfaces.UploadPercent
				err = json.Unmarshal(progMessage, &ulPct)
				if err != nil {
					return sharedInterfaces.UploadSuccess{}, fmt.Errorf("error unmarshalling upload progress message: %s", err)
				}
				progressChannel <- ulPct.Percent
			}
		}
	}
	return sharedInterfaces.UploadSuccess{}, nil
}
