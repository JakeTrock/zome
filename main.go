package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	ds "github.com/ipfs/go-datastore"

	"github.com/adrg/xdg"
	si "github.com/jaketrock/zome/sharedInterfaces"
	"github.com/jaketrock/zome/zcrypto"
	crypto "github.com/libp2p/go-libp2p/core/crypto"

	badger "github.com/ipfs/go-ds-badger"
	logging "github.com/ipfs/go-log/v2"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const name = "zome"
const zomeVersion = "0.0.1"

type App struct {
	ctx           context.Context
	operatingPath string
	startTime     time.Time
	Logger        *logging.ZapEventLogger

	dbCryptKey []byte
	store      *badger.Datastore
	host       host.Host
	connTopic  *pubsub.Topic //all of your zome nodes share across this topic

	fsActiveWrites map[string]si.UploadHeader //TODO: reading and writing to maps is not threadsafe
	fsActiveReads  map[string]si.DownloadHeader

	peerId     peer.ID
	privateKey crypto.PrivKey

	friendlyName string
	maxSpace     int64 //TODO: useme
}

func initPath(overridePath string) string {
	var configFilePath string
	if overridePath != "" {
		log.Println("Using config path override")
		configFilePath = overridePath
		statpath, err := os.Stat(overridePath)
		log.Println(statpath)
		if os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Join(overridePath, name), 0755)
			if err != nil {
				log.Fatal(err)
			}
		}
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Println("Using default config path")
		configFilePath = filepath.Join(xdg.ConfigHome, name)
	}
	return configFilePath
}

func retrievePrivateKey(store *badger.Datastore, ctx context.Context) crypto.PrivKey {
	var priv crypto.PrivKey
	k := ds.NewKey("userKey")
	v, err := store.Get(ctx, k)
	if err != nil && err != ds.ErrNotFound {
		log.Fatal(err)
	} else if v != nil {
		log.Println("PrivateKey exists, retrieving...")
		priv, err = crypto.UnmarshalPrivateKey(v)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Println("PrivateKey doesn't exist, generating...")
		priv, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 1)
		if err != nil {
			log.Fatal(err)
		}
		data, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			log.Fatal(err)
		}
		err = store.Put(ctx, k, data)
		if err != nil {
			log.Fatal(err)
		}
	}
	return priv
}

func retrieveDbKey(refPath string) []byte {
	keyPath := filepath.Join(refPath, "key")
	var priv []byte
	_, err := os.Stat(keyPath)
	if os.IsNotExist(err) {
		priv, err = zcrypto.NewAESKey()
		if err != nil {
			log.Fatal(err)
		}
		err = os.WriteFile(keyPath, priv, fs.FileMode(0400))
		if err != nil {
			log.Fatal(err)
		}
	} else if err != nil {
		log.Fatal(err)
	} else {
		priv, err = os.ReadFile(keyPath)
		if err != nil {
			log.Fatal(err)
		}
	}
	return priv
}

func (a *App) Startup(overrides map[string]string) {
	crypto.MinRsaKeyBits = 1024

	a.Logger = logging.Logger("zome")

	logging.SetLogLevel("*", "error")
	ctx := context.Background()

	configFilePath := initPath(overrides["configPath"])

	data := filepath.Join(configFilePath, name)

	store, err := badger.NewDatastore(data, &badger.DefaultOptions)
	if err != nil {
		a.Logger.Fatal(err)
	}

	priv := retrievePrivateKey(store, ctx)

	pid, err := peer.IDFromPublicKey(priv.GetPublic())
	if err != nil {
		a.Logger.Fatal(err)
	}

	a.friendlyName = "zome"
	v, err := store.Get(ctx, ds.NewKey("friendlyName")) //get the user friendly name
	if err != nil && err != ds.ErrNotFound {
		a.Logger.Fatal(err)
	} else if v != nil {
		a.friendlyName = string(v)
	}

	a.ctx = ctx
	a.operatingPath = configFilePath

	a.startTime = time.Now() //we dispose of this on shutdown
	a.peerId = pid
	a.privateKey = priv
	a.dbCryptKey = retrieveDbKey(data) // file based key, can be moved to lock db

	a.fsActiveWrites = make(map[string]si.UploadHeader)
	a.fsActiveReads = make(map[string]si.DownloadHeader)
	a.store = store
}

func (a *App) Shutdown() {
	a.store.Close()
}

func main() {
	configPathOverride := flag.String("configPath", "", "overrides default config path")
	// configPortOverride := flag.String("port", "", "overrides default port")
	help := flag.Bool("h", false, "Display Help")
	flag.Parse()
	if *help {
		log.Println("Zome under construction")
		flag.PrintDefaults()
		return
	}

	app := &App{}

	app.Startup(map[string]string{"configPath": *configPathOverride})

	app.InitP2P()

	app.initInterface()

	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		log.Println("Running in daemon mode")
		go func() {
			for {
				log.Printf("%s - %d connected peers\n", time.Now().Format(time.Stamp), len(connectedPeersFull(app.host)))
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
