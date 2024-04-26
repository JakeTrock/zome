package libzome

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/libp2p/go-libp2p/core/network"
)

type ZomeDownloader struct {
	socket network.Stream
}

func (zd *ZomeDownloader) InitDownload(downloadId string) error {
	_, err := zd.socket.Write([]byte(downloadId))
	if err != nil {
		return fmt.Errorf("error writing downloadId: %s", err)
	}
	return nil
}

func (zd *ZomeDownloader) ReadWholeFile(downloadInto *os.File, ctrlChannel chan string, progressChannel chan int) (sharedInterfaces.DownloadSuccess, error) {
	// get expected size
	sizeStruct := struct {
		Code   int                                     `json:"code"`
		Status sharedInterfaces.DownloadInitialMessage `json:"status"`
	}{}
	var sizeJson []byte
	_, err := zd.socket.Read(sizeJson)
	if err != nil {
		return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("error reading size: %s", err)
	}
	err = json.Unmarshal(sizeJson, &sizeStruct)
	if err != nil {
		return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("error unmarshalling size: %s", err)
	}

	// pull chunks from download socket and feed them into the file
	for {
		// if we get a cancel message on the control channel, stop the process
		select {
		case cmsg := <-ctrlChannel:
			if cmsg == "CANCEL" {
				_, err = zd.socket.Write([]byte("CANCEL"))
				if err != nil {
					return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("error writing cancel message: %s", err)
				}
				return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("download cancelled")
			}
		default:
		}

		var chunk []byte
		_, err := zd.socket.Read(chunk)
		if err != nil {
			return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("error reading chunk: %s", err)
		}

		//check if we've written more than the expected size
		if int64(len(chunk)) > sizeStruct.Status.Size {
			return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("error: chunk size is larger than expected")
		}

		fi, err := downloadInto.Stat()
		if err != nil {
			return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("error getting file stats: %s", err)
		}
		fiSize := fi.Size()

		//check if we've written the expected size
		if fiSize == sizeStruct.Status.Size {
			break
		}

		_, err = downloadInto.Write(chunk)
		if err != nil {
			return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("error writing chunk: %s", err)
		}

		if fiSize+int64(len(chunk)) == sizeStruct.Status.Size {
			msgJson := struct {
				Code   int                              `json:"code"`
				Status sharedInterfaces.DownloadSuccess `json:"status"`
			}{}
			err = json.Unmarshal(chunk, &msgJson)
			if err != nil {
				return sharedInterfaces.DownloadSuccess{}, fmt.Errorf("error unmarshalling success message: %s", err)
			}
			return msgJson.Status, nil
		}
	}
	return sharedInterfaces.DownloadSuccess{}, nil
}
