package main

import (
	"math/rand"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	ds "github.com/ipfs/go-datastore"
)

type testContext struct {
	app           *App
	path          string
	requestDomain string //localhost:port
	originBase    string
}

const testPathBase = "./testPath"

var firstApp = testContext{
	app:           &App{},
	path:          path.Join(testPathBase, "/first"),
	requestDomain: "localhost:5252",
	originBase:    generateRandomKey() + ".trock.com",
}
var secondApp = testContext{
	app:           &App{},
	path:          path.Join(testPathBase, "/second"),
	requestDomain: "localhost:5253",
	originBase:    generateRandomKey() + ".crouton.net",
}

// setup tests
func (tc *testContext) setupSuite() {
	if tc.app.startTime.IsZero() {
		os.RemoveAll(path.Join(tc.path, "zome"))
		tc.app.Startup(map[string]string{"configPath": tc.path})
		tc.app.InitP2P()
		go func() {
			requestDomainPort := strings.Split(tc.requestDomain, ":")[1]
			tc.app.initWeb(map[string]string{"configPort": requestDomainPort})
		}()
		//enable fs and s3 global write
		err := tc.app.store.Put(tc.app.ctx, ds.NewKey(tc.originBase+"]-FACL"), []byte{1})
		if err != nil {
			logger.Error(err)
		}
		err = tc.app.store.Put(tc.app.ctx, ds.NewKey(tc.originBase+"]-GW"), []byte{1})
		if err != nil {
			logger.Error(err)
		}
	}
}

func TestMain(m *testing.M) {
	//clear the testPath

	os.RemoveAll(path.Join(testPathBase, "downloads"))
	firstApp.setupSuite()
	// go func() {
	// 	secondApp.setupSuite()  //TODO: why does this cause a crash?
	// 	// defer secondApp.app.Shutdown()
	// }()

	code := m.Run()
	// Perform any teardown or cleanup here
	firstApp.app.Shutdown()
	// secondApp.app.Shutdown()
	os.Exit(code)
}

func (tc *testContext) establishControlSocket() *websocket.Conn {
	header := http.Header{}
	header.Add("Origin", "https://"+tc.originBase)
	controlSocket, _, err := websocket.DefaultDialer.Dial("ws://"+tc.requestDomain+"/v1/control/", header)
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
