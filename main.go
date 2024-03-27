package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	logging "github.com/ipfs/go-log/v2"

	"github.com/adrg/xdg"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const name = "zome"
const zomeVersion = "0.0.1"

var logger = logging.Logger("globalLogs")

type App struct {
	ctx           context.Context
	operatingPath string
	startTime     time.Time

	dbCryptKey      []byte
	store           *badger.Datastore
	host            host.Host
	restrictedTopic *pubsub.Topic //all of your zome nodes share across this topic
	publicTopic     *pubsub.Topic //all of your app nodes share across this topic
	//TODO: add key listing of all peers in public topic, restricted topic
	//TODO: add listing of "friend" topics with which you share data, do this after you've developed the gui/file mgmt sys

	fsActiveWrites map[string]UploadHeader //TODO: reading and writing to maps is not threadsafe
	fsActiveReads  map[string]DownloadHeader

	peerId     peer.ID
	privateKey crypto.PrivKey

	friendlyName string
	maxSpace     int64 //TODO: useme
}

func initPath(overridePath string) string {
	var configFilePath string
	if overridePath != "" {
		logger.Info("Using config path override")
		configFilePath = overridePath
		statpath, err := os.Stat(overridePath)
		logger.Info(statpath)
		if os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Join(overridePath, name), 0755)
			if err != nil {
				log.Fatal(err)
			}
		}
		if err != nil {
			logger.Fatal(err)
		}
	} else {
		logger.Info("Using default config path")
		configFilePath = filepath.Join(xdg.ConfigHome, name)
	}
	return configFilePath
}

func retrievePrivateKey(store *badger.Datastore, ctx context.Context) crypto.PrivKey {
	var priv crypto.PrivKey
	k := ds.NewKey("userKey")
	v, err := store.Get(ctx, k)
	if err != nil && err != ds.ErrNotFound {
		logger.Fatal(err)
	} else if v != nil {
		logger.Info("priv exists")
		priv, err = crypto.UnmarshalPrivateKey(v)
		if err != nil {
			logger.Fatal(err)
		}
	} else {
		logger.Info("priv doesn't exist")
		priv, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 1)
		if err != nil {
			logger.Fatal(err)
		}
		data, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			logger.Fatal(err)
		}
		err = store.Put(ctx, k, data)
		if err != nil {
			logger.Fatal(err)
		}
	}
	return priv
}

func retrieveDbKey(refPath string) []byte {
	keyPath := filepath.Join(refPath, "key")
	var priv []byte
	_, err := os.Stat(keyPath)
	if os.IsNotExist(err) {
		priv, err = NewAESKey()
		if err != nil {
			logger.Fatal(err)
		}
		err = os.WriteFile(keyPath, priv, fs.FileMode(0400))
		if err != nil {
			logger.Fatal(err)
		}
	} else if err != nil {
		logger.Fatal(err)
	} else {
		priv, err = os.ReadFile(keyPath)
		if err != nil {
			logger.Fatal(err)
		}
	}
	return priv
}

func (a *App) Startup(overrides map[string]string) {
	crypto.MinRsaKeyBits = 1024

	logging.SetLogLevel("*", "error")
	ctx := context.Background()

	configFilePath := initPath(overrides["configPath"])

	data := filepath.Join(configFilePath, name)

	store, err := badger.NewDatastore(data, &badger.DefaultOptions)
	if err != nil {
		logger.Fatal(err)
	}

	priv := retrievePrivateKey(store, ctx)

	pid, err := peer.IDFromPublicKey(priv.GetPublic())
	if err != nil {
		logger.Fatal(err)
	}

	a.friendlyName = "zome"
	v, err := store.Get(ctx, ds.NewKey("friendlyName")) //get the user friendly name
	if err != nil && err != ds.ErrNotFound {
		logger.Fatal(err)
	} else if v != nil {
		a.friendlyName = string(v)
	}

	a.ctx = ctx
	a.operatingPath = configFilePath

	a.startTime = time.Now() //we dispose of this on shutdown
	a.peerId = pid
	a.privateKey = priv
	a.dbCryptKey = retrieveDbKey(data) // file based key, can be moved to lock db

	a.fsActiveWrites = make(map[string]UploadHeader)
	a.fsActiveReads = make(map[string]DownloadHeader)
	a.store = store
}

func (a *App) Shutdown() {
	a.store.Close()
}

func main() {
	configPathOverride := flag.String("configPath", "", "overrides default config path")
	configPortOverride := flag.String("port", "", "overrides default port")
	help := flag.Bool("h", false, "Display Help")
	flag.Parse()
	if *help {
		fmt.Println("zome under construction")
		flag.PrintDefaults()
		return
	}

	app := &App{}

	app.Startup(map[string]string{"configPath": *configPathOverride})

	app.InitP2P()

	app.initWeb(map[string]string{"configPort": *configPortOverride})

	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		logger.Info("Running in daemon mode")
		go func() {
			for {
				logger.Infof("%s - %d connected peers\n", time.Now().Format(time.Stamp), len(connectedPeersFull(app.host)))
				time.Sleep(10 * time.Second)
			}
		}()
		signalChan := make(chan os.Signal, 20)
		signal.Notify(
			signalChan,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGHUP,
		)
		<-signalChan
		return
	}

	defer app.store.Close()
	_, cancel := context.WithCancel(app.ctx)
	defer cancel()

}
