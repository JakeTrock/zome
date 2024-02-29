package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lucsky/cuid"

	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	logging "github.com/ipfs/go-log/v2"

	"github.com/adrg/xdg"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const name = "zome"

var logger = logging.Logger("globaldb")

type App struct {
	ctx           context.Context
	operatingPath string
	startTime     time.Time

	dbCryptKey []byte
	store      *badger.Datastore
	host       host.Host
	subTopics  map[string][]string

	peerId     peer.ID
	privateKey crypto.PrivKey
}

func initPath(overridePath string) string {
	var configFilePath string
	if overridePath != "" {
		fmt.Println("Using config path override")
		configFilePath = overridePath
		statpath, err := os.Stat(overridePath)
		print(statpath)
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
		fmt.Println("Using default config path")
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
		println("priv exists")
		priv, err = crypto.UnmarshalPrivateKey(v)
		if err != nil {
			logger.Fatal(err)
		}
	} else {
		println("priv doesn't exist")
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

func retrieveTopics(store *badger.Datastore, ctx context.Context) map[string][]string {
	topics := map[string][]string{}
	k := ds.NewKey("topics")
	v, err := store.Get(ctx, k)
	if err != nil && err != ds.ErrNotFound {
		logger.Fatal(err)
	} else if v != nil {
		json.Unmarshal(v, &topics)
		if err != nil {
			logger.Fatal(err)
		}
	} else {
		randomUUID := cuid.New()
		topics[randomUUID] = []string{}
		data, err := json.Marshal(topics)
		if err != nil {
			logger.Fatal(err)
		}
		err = store.Put(ctx, k, data)
		if err != nil {
			logger.Fatal(err)
		}

	}
	return topics
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

	data := filepath.Join(configFilePath, name+"-data")

	store, err := badger.NewDatastore(data, &badger.DefaultOptions)
	if err != nil {
		logger.Fatal(err)
	}

	priv := retrievePrivateKey(store, ctx)

	pid, err := peer.IDFromPublicKey(priv.GetPublic())
	if err != nil {
		logger.Fatal(err)
	}

	a.subTopics = retrieveTopics(store, ctx)

	a.ctx = ctx
	a.operatingPath = configFilePath

	a.startTime = time.Now() //we dispose of this on shutdown
	a.peerId = pid
	a.privateKey = priv
	a.dbCryptKey = retrieveDbKey(data) // file based key, can be moved to lock db

	a.store = store
}

func main() {
	configPathOverride := flag.String("configPath", "", "overrides default config path")
	help := flag.Bool("h", false, "Display Help")
	flag.Parse()
	if *help {
		fmt.Println("zome under construction")
		fmt.Println()
		flag.PrintDefaults()
		return
	}

	app := &App{}

	app.Startup(map[string]string{"configPath": *configPathOverride})

	app.InitP2P()

	app.initWeb()

	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		fmt.Println("Running in daemon mode")
		go func() {
			for {
				fmt.Printf("%s - %d connected peers\n", time.Now().Format(time.Stamp), len(connectedPeers(app.host)))
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
