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
	"strconv"
	"strings"
	"syscall"
	"time"

	ds "github.com/ipfs/go-datastore"
	"github.com/lucsky/cuid"

	"github.com/adrg/xdg"
	libzome "github.com/jaketrock/zome/libZome"
	"github.com/jaketrock/zome/sharedInterfaces"
	si "github.com/jaketrock/zome/sharedInterfaces"
	"github.com/jaketrock/zome/zcrypto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	crypto "github.com/libp2p/go-libp2p/core/crypto"

	badger "github.com/ipfs/go-ds-badger"
	logging "github.com/ipfs/go-log/v2"

	"github.com/libp2p/go-libp2p/core/peer"
)

type App struct {
	ctx           context.Context
	operatingPath string
	startTime     time.Time
	Logger        *logging.ZapEventLogger

	dbCryptKey []byte
	store      *badger.Datastore
	network    *zcrypto.Zstate

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
			err = os.MkdirAll(filepath.Join(overridePath, sharedInterfaces.AppNameSpace), 0755)
			if err != nil {
				log.Fatal(err)
			}
		}
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Println("Using default config path")
		configFilePath = filepath.Join(xdg.ConfigHome, sharedInterfaces.AppNameSpace)
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

func (a *App) retrieveTopic(connOverride string) string {
	var topicName string

	if connOverride != "" {
		topicName = connOverride
	} else {
		hTopicName := "herdTopic"
		herdTopic, err := a.secureInternalKeyGet(hTopicName)
		topicName = string(herdTopic)
		if err != nil && err != ds.ErrNotFound {
			a.Logger.Fatal(err)
		} else if err == ds.ErrNotFound {
			randomUUID := cuid.New()

			err = a.secureInternalKeyAdd(hTopicName, []byte(randomUUID))
			if err != nil {
				a.Logger.Fatal(err)
			}
			topicName = randomUUID
		}
	}
	a.Logger.Info("P2P topic: ", topicName)
	return topicName
}

func (a *App) Startup(overrides map[string]string) {
	crypto.MinRsaKeyBits = 1024

	a.Logger = logging.Logger("zome")

	logging.SetLogLevel("*", "error")
	ctx := context.Background()

	configFilePath := initPath(overrides["configPath"])

	store, err := badger.NewDatastore(configFilePath, &badger.DefaultOptions)
	if err != nil {
		a.Logger.Fatal(err)
	}

	priv := retrievePrivateKey(store, ctx)

	pid, err := peer.IDFromPublicKey(priv.GetPublic())
	if err != nil {
		a.Logger.Fatal(err)
	}

	a.ctx = ctx
	a.operatingPath = configFilePath

	a.startTime = time.Now() //we dispose of this on shutdown
	a.peerId = pid
	fmt.Println("Peer ID: ", a.peerId)
	a.privateKey = priv
	a.dbCryptKey = retrieveDbKey(configFilePath) // file based key, can be moved to lock db

	a.fsActiveWrites = make(map[string]si.UploadHeader)
	a.fsActiveReads = make(map[string]si.DownloadHeader)
	a.store = store
}

func (a *App) Shutdown() {
	a.store.Close()
}

func (a *App) updateInformation() {
	err := a.network.AddStoreInfo("name", a.friendlyName) //user friendly name
	if err != nil {
		a.Logger.Error(err)
	}
	err = a.network.AddStoreInfo("start", a.startTime.String()) //TODO: implement server-to-server, s2c info sync
	if err != nil {
		a.Logger.Error(err)
	}
	dataDirSize, err := zcrypto.DirSize(a.operatingPath)
	if err != nil {
		a.Logger.Error(err)
	}
	dbSize, err := ds.DiskUsage(a.ctx, a.store)
	if err != nil {
		a.Logger.Error(err)
	}
	a.network.AddStoreInfo("space", strconv.FormatInt(dataDirSize+int64(dbSize), 10))
}

func main() {
	configPathOverride := flag.String("configPath", "", "overrides default config path")
	connTopicOverride := flag.String("connTopic", "", "overrides internal connect topic")
	lZomeOnly := flag.String("lzoid", "", "only run libZome, specify id of conn") //TODO: remove, for testing
	help := flag.Bool("h", false, "Display Help")
	flag.Parse()
	if *help {
		flag.PrintDefaults()
		return
	}

	if *lZomeOnly != "" {
		fmt.Println("starting libzome only")
		libzome.Initialize(*connTopicOverride, "")
		peerList := libzome.ListPeers()
		if len(peerList) == 0 {
			fmt.Println("no peers found lzo")
		}
		fmt.Println(peerList)
		servConn, err := libzome.WaitForConnection(*connTopicOverride, *lZomeOnly)
		if err != nil {
			fmt.Println(err)
		}
		spid := libzome.GetSelfId(*connTopicOverride)
		fmt.Println(spid)
		gwr, err := servConn.DbGetGlobalWrite()
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(gwr)
		gfw, err := servConn.FsGetGlobalWrite()
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(gfw)
		for {
			time.Sleep(100 * time.Millisecond)
		}
	}

	app := &App{}

	app.Startup(map[string]string{
		"configPath": *configPathOverride,
	})

	zstate := zcrypto.Zstate{
		Ctx: app.ctx,
	}
	err := zstate.InitP2P(app.privateKey)
	if err != nil {
		app.Logger.Fatal(err)
	}
	app.network = &zstate
	defer zstate.Close()

	//TODO: multiretrieve
	topic := app.retrieveTopic(*connTopicOverride)
	zstate.AddCommTopic(topic, func(msg *pubsub.Message) {
		app.Logger.Info("Received message: ", string(msg.Data))
	})

	cancelComms, err := app.initCRDT(topic)
	if err != nil {
		app.Logger.Fatal(err)
	}
	defer cancelComms()
	//=========

	//make new badger store around crdt

	app.initInterface()

	app.friendlyName = "zomereplica"
	//set the friendly name
	v, err := app.secureInternalKeyGet("friendlyName")
	if err != nil && err != ds.ErrNotFound {
		app.Logger.Fatal(err)
	} else if err == ds.ErrNotFound || v == nil || string(v) == "" {
		pid, err := peer.IDFromPublicKey(app.privateKey.GetPublic())
		if err != nil {
			app.Logger.Fatal(err)
		}
		fnm, err := sharedInterfaces.AnimalHash(pid.String())
		if err != nil {
			app.Logger.Fatal(err)
		}
		err = app.secureInternalKeyAdd("friendlyName", []byte(fnm))
		if err != nil {
			app.Logger.Fatal(err)
		}
		app.friendlyName = fnm
	} else {
		app.friendlyName = string(v)
	}

	// run in daemon mode
	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		log.Println("Running in daemon mode")
		go func() {
			for {
				log.Printf("%s - %d connected peers\n", time.Now().Format(time.Stamp), len(zstate.ConnectedPeersFull([]string{})))
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

	app.network.AddStoreInfo("serverVersion", sharedInterfaces.AppNameSpace)

	infoSubrange := 0
	// run indefinitely
	for {
		select {
		case <-app.ctx.Done():
			return
		default:
			app.network.PingAllPeers(topic)

			if infoSubrange > 100 {
				fmt.Println("pst: " + strings.Join(app.network.Psub.GetTopics(), ", "))
				app.updateInformation()
				infoSubrange = 0
			} else {
				infoSubrange++
			}

			cpc := app.network.ConnectedPeersClean([]string{topic})
			if len(cpc) != 0 {
				fmt.Println("Connected peers:")
				for _, p := range cpc {
					fmt.Println(p)
				}
			}
			//loop sending info

			time.Sleep(100 * time.Millisecond)
		}
	}

}
