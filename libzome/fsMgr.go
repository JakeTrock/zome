package libzome

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"

	guuid "github.com/google/uuid"
	"golang.org/x/mod/sumdb/dirhash"

	kyberk2so "github.com/symbolicsoft/kyber-k2so"

	"github.com/adrg/xdg"
)

func (a *App) LoadConfig(overrides map[string]string) {
	configFilePath, err := xdg.SearchConfigFile("zome/config.json")

	if configFilePath == "" || err != nil { //https://github.com/adrg/xdg
		fmt.Println("Creating new config file")
		if err != nil {
			fmt.Println(err)
		}
		configFilePath, err := xdg.ConfigFile("/zome/config.json")
		if err != nil {
			log.Fatal("Error when creating file: ", err)
		}
		fmt.Println(configFilePath)
		uuid := guuid.New().String()
		if overrides["uuid"] != "" {
			uuid = overrides["uuid"]
		}

		poolId := guuid.New().String()
		if overrides["poolId"] != "" {
			poolId = overrides["poolId"]
		}

		privateKey, publicKey, _ := kyberk2so.KemKeypair768()
		pickleConfig := ConfigPickled{
			uuid,
			poolId,
			"Anonymous",
			base64.StdEncoding.EncodeToString(publicKey[:]),
			base64.StdEncoding.EncodeToString(privateKey[:]),
			make(map[string]string),
			[]string{},
		}
		fmt.Println(pickleConfig)
		pickleConfigBytes, _ := json.Marshal(pickleConfig)
		err = os.WriteFile(configFilePath, pickleConfigBytes, 0644)
		if err != nil {
			log.Fatal("Error when writing file: ", err)
		}
		a.globalConfig = ConfigObject{
			uuid,
			poolId,
			"Anonymous",
			publicKey,
			privateKey,
			make(map[string][1184]byte),
			[]string{},
		}
	} else {
		fmt.Println("Loading existing config file")
		//load config
		cfile, err := os.ReadFile(configFilePath)
		if err != nil {
			log.Fatal("Error when opening file: ", err)
		}
		var cfgPickle ConfigPickled
		err = json.Unmarshal(cfile, &cfgPickle)
		fmt.Println(cfgPickle)

		publicKeyBytes, _ := base64.StdEncoding.DecodeString(cfgPickle.PubKey64)
		privateKeyBytes, _ := base64.StdEncoding.DecodeString(cfgPickle.PrivKey64)
		unpickledKeypairs := make(map[string][1184]byte)
		for k, v := range cfgPickle.KnownKeypairs {
			publicKeyBytes, _ := base64.StdEncoding.DecodeString(v)
			var publicKey [1184]byte
			copy(publicKey[:], publicKeyBytes)
			unpickledKeypairs[k] = publicKey
		}
		uuid := cfgPickle.Uuid
		if overrides["uuid"] != "" {
			uuid = overrides["uuid"]
		}
		poolId := cfgPickle.PoolId
		if overrides["poolId"] != "" {
			poolId = overrides["poolId"]
		}
		a.globalConfig = ConfigObject{
			uuid,
			poolId,
			cfgPickle.UserName,
			[1184]byte(publicKeyBytes),
			[2400]byte(privateKeyBytes),
			unpickledKeypairs,
			cfgPickle.EnabledPlugins,
		}
		if err != nil {
			log.Fatal("Error during Unmarshal(): ", err)
		}
	}
}

// TODO: plugin import logic
func (a *App) RefreshPlugins() {
	uuid := a.globalConfig.uuid
	okPlugins := a.globalConfig.enabledPlugins
	newPlugins := make(map[string]string)

	configFilePath := xdg.ConfigHome + "zome/plugins-" + uuid
	subfolders, err := os.ReadDir(configFilePath)
	if err != nil {
		log.Fatal(err)
	}
	for _, fileEntry := range subfolders {
		if fileEntry.IsDir() {
			hash, err := dirhash.HashDir(configFilePath+"/"+fileEntry.Name(), "", dirhash.Hash1)
			if err != nil {
				log.Fatal(err)
			}
			if hash != "" {
				log.Fatal("Error: empty hash")
			}
			newPlugins[hash] = fileEntry.Name()
		}
	}
	for _, plugin := range okPlugins {
		if pluginPath, ok := newPlugins[plugin]; ok {
			//TODO: load plugin from path
			fmt.Println(pluginPath)
		}
	}
}
