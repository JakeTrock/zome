package main

import (
	"fmt"
	"os"
	"path"
	"testing"

	ds "github.com/ipfs/go-datastore"
	libzome "github.com/jaketrock/zome/libZome"
	"github.com/jaketrock/zome/zcrypto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type testContext struct {
	app        *App
	path       string
	originBase string
	servConn   libzome.ZomeController
}

const testPathBase = "./testPath"
const unifiedTopic = "zomeUnitTests"

var firstApp = testContext{
	app:  &App{},
	path: path.Join(testPathBase, "/first"),
}
var secondApp = testContext{
	app:  &App{},
	path: path.Join(testPathBase, "/second"),
}

// setup tests
func (tc *testContext) setupSuite() error {
	if tc.app.startTime.IsZero() {
		os.RemoveAll(path.Join(tc.path, "zome"))
		pathOverride := map[string]string{"configPath": tc.path}
		tc.app.Startup(pathOverride)
		topic := tc.app.retrieveTopic(unifiedTopic)

		zstate := zcrypto.Zstate{
			Ctx: tc.app.ctx,
		}
		err := zstate.InitP2P(tc.app.privateKey)
		if err != nil {
			tc.app.Logger.Fatal(err)
		}
		tc.app.network = &zstate
		defer zstate.Close()

		//TODO: multiretrieve
		zstate.AddCommTopic(topic, func(msg *pubsub.Message) {
			tc.app.Logger.Info("Received message: ", string(msg.Data))
		})

		cancelComms, err := tc.app.initCRDT(topic)
		if err != nil {
			tc.app.Logger.Fatal(err)
		}
		defer cancelComms()

		tc.app.initInterface()

		libzome.Initialize(topic, "")

		spid := libzome.GetSelfId(topic)
		tc.originBase = spid
		//enable fs and s3 global write
		err = tc.app.store.Put(tc.app.ctx, ds.NewKey(spid+"]-FACL"), []byte{1})
		if err != nil {
			fmt.Println(err)
		}
		err = tc.app.store.Put(tc.app.ctx, ds.NewKey(spid+"]-GW"), []byte{1})
		if err != nil {
			fmt.Println(err)
		}
		zc, err := libzome.WaitForConnection(topic, spid)
		if err != nil {
			tc.app.Logger.Fatal(err)
		}
		tc.servConn = zc
	}
	return nil
}

func TestMain(m *testing.M) {
	//clear the testPath

	os.RemoveAll(path.Join(testPathBase, "downloads"))
	err := firstApp.setupSuite()
	if err != nil {
		fmt.Println(err)
		return
	}
	// go func() {
	// 	secondApp.setupSuite()  //TODO: crossorigin tests
	// 	// defer secondApp.app.Shutdown()
	// }()

	code := m.Run()
	// Perform any teardown or cleanup here
	firstApp.app.Shutdown()
	// secondApp.app.Shutdown()
	os.Exit(code)
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
	single := zcrypto.GenerateRandomKey()
	randomKeyValuePairs := keyValueReq{
		values: map[string]string{
			zcrypto.GenerateRandomKey(): zcrypto.GenerateRandomKey(),
			zcrypto.GenerateRandomKey(): zcrypto.GenerateRandomKey(),
		},
	}

	randomKeyValuePairs.values[single] = zcrypto.GenerateRandomKey()

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
// 				decryptedValue, err := zcrypto.AesGCMDecrypt(a.dbCryptKey, val[2:])
// 				if err != nil {
// 					a.Logger.Error(err)
// 					return err
// 				}

// 				// Use the decrypted value
// 				fmt.Println("value: " + string(decryptedValue))

// 				return nil
// 			})
// 			if err != nil {
// 				a.Logger.Error(err)
// 				return err
// 			}
// 		}

// 		return nil
// 	})

// 	if err != nil {
// 		a.Logger.Error(err)
// 		return
// 	}
// }
