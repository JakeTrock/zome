package libzome

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"

	guuid "github.com/google/uuid"
	"golang.org/x/mod/sumdb/dirhash"

	kyberk2so "github.com/symbolicsoft/kyber-k2so"

	"strings"

	"github.com/adrg/xdg"
	imohash "github.com/kalafut/imohash"
	cpfd "github.com/u-root/u-root/pkg/cp"
)

func isBadPath(path string) bool {
	if path == "" {
		return true
	} else if strings.Contains(path, "..") {
		return true
	} else if strings.Contains(path, "~") {
		return true
	}
	return false
}

func createConfigFile(cfPath string) {
	fmt.Println("Creating new config file")

	fmt.Println(cfPath)
	uuid := guuid.New().String()
	poolId := guuid.New().String()

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
	err := os.WriteFile(cfPath, pickleConfigBytes, 0644)
	if err != nil {
		log.Fatal("Error when writing file: ", err)
	}
}

func (a *App) FsLoadConfig(overrides map[string]string) { //https://github.com/adrg/xdg
	var configFilePath string = ""
	var err error = nil

	if overrides["configPath"] != "" {
		fmt.Println("Using config path override")
		configFilePath = overrides["configPath"] + "/zome/config.json"
		//check if dir exists
		if overrides["configPath"] != "" {
			statpath, err := os.Stat(overrides["configPath"] + "/zome")
			if os.IsNotExist(err) {
				err = os.MkdirAll(overrides["configPath"]+"/zome", 0755)
				if err != nil {
					log.Fatal(err)
				}
				newPath, err := os.Create(configFilePath)
				if err != nil {
					log.Fatal(err)
				}
				createConfigFile(newPath.Name())
			}
			a.cfgPath = statpath.Name()
		} else {
			log.Fatal("Bad custom path")
		}
	} else {
		fmt.Println("Using default config path")
		configFilePath, err = xdg.SearchConfigFile("zome/config.json")
		if err != nil {
			fmt.Println(err)
			configFilePath, err = xdg.ConfigFile("/zome/config.json")
			a.cfgPath = configFilePath[:len(configFilePath)-12]
			if err != nil {
				log.Fatal("Error when creating file: ", err)
			}
			createConfigFile(configFilePath)
		}
	} // make path override better

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

func (a *App) FsCreateSandboxFolder(appId string) (bool, error) {
	uuid := a.globalConfig.uuid
	configFilePath := a.cfgPath + "/files-" + uuid + "/" + appId
	err := os.MkdirAll(configFilePath, 0755)
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	return true, nil
}

func (a *App) FsGetHash(appId string, subPath string, isFolder bool) (string, error) {
	uuid := a.globalConfig.uuid

	configFilePath := a.cfgPath + "/files-" + uuid + "/" + appId + "/" + subPath

	if isBadPath(configFilePath) {
		return "", fmt.Errorf("bad path(cannot contain .. or ~)")
	}
	var err error
	var hash string
	if isFolder {
		hash, err = dirhash.HashDir(configFilePath, "", dirhash.Hash1)
	} else {
		ih := imohash.New()
		var ihSums [16]byte
		ihSums, err = ih.SumFile(configFilePath)
		hash = hex.EncodeToString(ihSums[:])
	}
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return hash, nil
}

func (a *App) FsGetDirectoryListing(appId string, subPath string, includeHash bool) (string, error) {
	uuid := a.globalConfig.uuid
	refFilePath := a.cfgPath + "/files-" + uuid + "/" + appId + "/" + subPath
	if isBadPath(refFilePath) {
		return "", fmt.Errorf("bad path(cannot contain .. or ~)")
	}
	dirContents, err := os.ReadDir(refFilePath)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	fileStrings := make([]string, len(dirContents))
	for i, fileEntry := range dirContents {
		fileInfo, err := fileEntry.Info()
		if err != nil {
			fmt.Println(err)
			return "", err
		}
		if includeHash {
			if fileEntry.IsDir() {
				hash, err := a.FsGetHash(appId, subPath+"/"+fileEntry.Name(), false)
				if err != nil {
					return "", err
				}
				fileStrings[i] = fmt.Sprintf(`{"name": "%s", "isDir": %t, "size": %d, "mode": "%s", "modTime": "%s", "hash": "%s"}`,
					fileEntry.Name(), fileEntry.IsDir(), fileInfo.Size(), fileInfo.Mode(), fileInfo.ModTime(), hash)
			} else {
				hash, err := a.FsGetHash(appId, subPath+"/"+fileEntry.Name(), true)
				if err != nil {
					return "", err
				}
				fileStrings[i] = fmt.Sprintf(`{"name": "%s", "isDir": %t, "size": %d, "mode": "%s", "modTime": "%s", "hash": "%s"}`,
					fileEntry.Name(), fileEntry.IsDir(), fileInfo.Size(), fileInfo.Mode(), fileInfo.ModTime(), hash)
			}
		} else {
			fileStrings[i] = fmt.Sprintf(`{"name": "%s", "isDir": %t, "size": %d, "mode": "%s", "modTime": "%s"}`,
				fileEntry.Name(), fileEntry.IsDir(), fileInfo.Size(), fileInfo.Mode(), fileInfo.ModTime())
		}
	}
	return fmt.Sprintf("[%s]", strings.Join(fileStrings, ",")), nil
}

func (a *App) FsCreateFolder(appId string, folder string) (bool, error) {
	uuid := a.globalConfig.uuid
	refFilePath := a.cfgPath + "/files-" + uuid + "/" + appId + "/" + folder
	if isBadPath(refFilePath) {
		return false, fmt.Errorf("bad path(cannot contain .. or ~)")
	}
	err := os.MkdirAll(refFilePath, 0644)
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	return true, nil
}

func (a *App) FsDeleteFileOrFolder(appId string, files []string, isFolder []bool) (bool, error) { //TODO: the whole isfolder thing should go, it's kinda bad, we can detect this
	uuid := a.globalConfig.uuid
	refFilePath := a.cfgPath + "/files-" + uuid + "/" + appId + "/"
	if isBadPath(refFilePath) {
		return false, fmt.Errorf("bad path(cannot contain .. or ~)")
	}
	for r, file := range files {
		tmpRef := refFilePath + file
		var err error
		if isFolder[r] {
			err = os.RemoveAll(tmpRef)
		} else {
			err = os.Remove(tmpRef)
		}
		if err != nil {
			fmt.Println(err)
			return false, err
		}
	}
	return true, nil
}

func (a *App) FsMoveFileOrFolder(appId string, oldPath string, newPath string) (bool, error) {
	uuid := a.globalConfig.uuid
	oldFilePath := a.cfgPath + "/files-" + uuid + "/" + appId + "/" + oldPath
	newFilePath := a.cfgPath + "/files-" + uuid + "/" + appId + "/" + newPath
	if isBadPath(oldFilePath) || isBadPath(newFilePath) {
		return false, fmt.Errorf("bad path(cannot contain .. or ~)")
	}
	err := os.Rename(oldFilePath, newFilePath)
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	return true, nil
}

func (a *App) FsCopyFileOrFolder(appId string, oldPath string, newPath string, isFolder bool) (bool, error) { //TODO: switch to copyFS https://github.com/golang/go/issues/62484
	uuid := a.globalConfig.uuid
	oldFilePath := a.cfgPath + "/files-" + uuid + "/" + appId + "/" + oldPath
	newFilePath := a.cfgPath + "/files-" + uuid + "/" + appId + "/" + newPath
	if isBadPath(oldFilePath) || isBadPath(newFilePath) {
		return false, fmt.Errorf("bad path(cannot contain .. or ~)")
	}
	var err error
	if isFolder {
		err = cpfd.CopyTree(oldFilePath, newFilePath)
	} else {
		err = cpfd.Copy(oldFilePath, newFilePath)
	}
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	return true, nil
}

//TODO: upload/download file logic

//TODO: p2p file transfer logic(encrypted)

// TODO: plugin import logic
func (a *App) RefreshPlugins() {
	uuid := a.globalConfig.uuid
	okPlugins := a.globalConfig.enabledPlugins
	newPlugins := make(map[string]string)

	configFilePath := a.cfgPath + "/plugins-" + uuid
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
