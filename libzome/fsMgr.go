package libzome

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
	"golang.org/x/mod/sumdb/dirhash"

	kyberk2so "github.com/symbolicsoft/kyber-k2so"

	"github.com/adrg/xdg"
)

func (a *App) loadConfig() {
	configFilePath, err := xdg.SearchConfigFile("zome/config.json")
	if err != nil {
		logger.Error(err)
	}

	if configFilePath == "" { //https://github.com/adrg/xdg
		configFilePath := xdg.ConfigHome + "zome/config.json"
		uuid := uuid.New().String()
		privateKey, publicKey, _ := kyberk2so.KemKeypair768()
		pickleConfig := ConfigPickled{
			uuid,
			"Anonymous",
			base64.StdEncoding.EncodeToString(publicKey[:]),
			base64.StdEncoding.EncodeToString(privateKey[:]),
			make(map[string]string),
			[]string{},
		}
		pickleConfigBytes, _ := json.Marshal(pickleConfig)
		os.WriteFile(configFilePath, pickleConfigBytes, 0644)

		a.globalConfig = ConfigObject{
			uuid,
			"Anonymous",
			publicKey,
			privateKey,
			make(map[string][1184]byte),
			[]string{},
		}
	} else {
		//load config
		cfile, err := os.ReadFile(configFilePath)
		if err != nil {
			logger.Error("Error when opening file: ", err)
		}
		var cfgPickle ConfigPickled
		err = json.Unmarshal(cfile, &cfgPickle)
		publicKeyBytes, _ := base64.StdEncoding.DecodeString(cfgPickle.PubKey64)
		privateKeyBytes, _ := base64.StdEncoding.DecodeString(cfgPickle.PrivKey64)
		unpickledKeypairs := make(map[string][1184]byte)
		for k, v := range cfgPickle.knownKeypairs {
			publicKeyBytes, _ := base64.StdEncoding.DecodeString(v)
			var publicKey [1184]byte
			copy(publicKey[:], publicKeyBytes)
			unpickledKeypairs[k] = publicKey
		}
		a.globalConfig = ConfigObject{
			cfgPickle.uuid,
			cfgPickle.userName,
			[1184]byte(publicKeyBytes),
			[2400]byte(privateKeyBytes),
			unpickledKeypairs,
			cfgPickle.enabledPlugins,
		}
		if err != nil {
			logger.Error("Error during Unmarshal(): ", err)
		}
	}
}

// TODO: plugin import logic
func (a *App) refreshPlugins() {
	uuid := a.globalConfig.uuid
	okPlugins := a.globalConfig.enabledPlugins
	newPlugins := make(map[string]string)

	configFilePath := xdg.ConfigHome + "p2dav/plugins-" + uuid
	subfolders, err := os.ReadDir(configFilePath)
	if err != nil {
		logger.Error(err)
	}
	for _, fileEntry := range subfolders {
		if fileEntry.IsDir() {
			hash, err := dirhash.HashDir(configFilePath+"/"+fileEntry.Name(), "", dirhash.Hash1)
			if err != nil {
				logger.Error(err)
			}
			if hash != "" {
				logger.Error("Error: empty hash")
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
