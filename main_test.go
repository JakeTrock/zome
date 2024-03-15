package main

import (
	"math/rand"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/gorilla/websocket"
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