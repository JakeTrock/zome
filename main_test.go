package main

import (
	"math/rand"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
)

var app = &App{}

const testPath = "./testPath"

const controlEndpoint = "ws://localhost:5253/v1/control/"

var originBase = generateRandomKey() + ".trock.com"
var originVar = "https://" + originBase

// setup tests
func setupSuite() {
	if app.startTime.IsZero() {
		app.Startup(map[string]string{"configPath": testPath})
		app.InitP2P()
		go func() {
			app.initWeb()
		}()
		//enable fs and s3 global write
		err := app.store.Put(app.ctx, ds.NewKey(originBase+"]-FACL"), []byte{1})
		if err != nil {
			logger.Error(err)
		}
		err = app.store.Put(app.ctx, ds.NewKey(originBase+"]-GW"), []byte{1})
		if err != nil {
			logger.Error(err)
		}
	}
}

func TestMain(m *testing.M) {
	//clear the testPath

	os.RemoveAll(path.Join(testPath, "zome"))
	os.RemoveAll(path.Join(testPath, "downloads"))
	setupSuite()
	code := m.Run()
	// Perform any teardown or cleanup here
	app.Shutdown()
	os.Exit(code)
}

func establishControlSocket() *websocket.Conn {
	header := http.Header{}
	header.Add("Origin", originVar)
	controlSocket, _, err := websocket.DefaultDialer.Dial(controlEndpoint, header)
	if err != nil {
		logger.Error(err)
		return nil
	}
	return controlSocket
}

func generateRandomKey() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 5)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

type keyValueReq struct {
	values map[string]string
}

type listReq struct {
	values []string
}

type singleReq struct {
	value string
}

func generateRandomStructs() (keyValueReq, listReq, singleReq) {
	single := generateRandomKey()
	randomKeyValuePairs := keyValueReq{
		values: map[string]string{
			generateRandomKey(): generateRandomKey(),
			generateRandomKey(): generateRandomKey(),
		},
	}

	randomKeyValuePairs.values[single] = generateRandomKey()

	randomList := listReq{
		values: []string{},
	}

	for k := range randomKeyValuePairs.values {
		randomList.values = append(randomList.values, k)
	}

	randomSingle := singleReq{
		value: single,
	}

	return randomKeyValuePairs, randomList, randomSingle
}

// func dumpDb(a *App) {
// 	fmt.Println("Dumping db")
// 	//get all decrypted values of badger datastore
// 	err := a.store.DB.View(func(txn *badger.Txn) error {
// 		opts := badger.DefaultIteratorOptions
// 		opts.PrefetchValues = true
// 		it := txn.NewIterator(opts)
// 		defer it.Close()

// 		for it.Rewind(); it.Valid(); it.Next() {
// 			item := it.Item()
// 			kCopyTo := make([]byte, len(item.Key()))
// 			item.KeyCopy(kCopyTo)
// 			fmt.Println("key: " + string(kCopyTo))
// 			err := item.Value(func(val []byte) error {
// 				// Decrypt the value here
// 				decryptedValue, err := AesGCMDecrypt(a.dbCryptKey, val[2:])
// 				if err != nil {
// 					logger.Error(err)
// 					return err
// 				}

// 				// Use the decrypted value
// 				fmt.Println("value: " + string(decryptedValue))

// 				return nil
// 			})
// 			if err != nil {
// 				logger.Error(err)
// 				return err
// 			}
// 		}

// 		return nil
// 	})

// 	if err != nil {
// 		logger.Error(err)
// 		return
// 	}
// }
