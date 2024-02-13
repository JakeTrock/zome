package libzome

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/adrg/xdg"
	bolt "go.etcd.io/bbolt"
)

const dbPath string = "/zome/main.db"

func (a *App) DbInit(overrides map[string]string) {
	var configFilePath string = ""
	var err error = nil

	if overrides["configPath"] != "" {
		fmt.Println("Using config path override")
		configFilePath = overrides["configPath"] + dbPath
	} else {
		fmt.Println("Using default config path")
		configFilePath, err = xdg.SearchConfigFile(dbPath)
	}

	if configFilePath == "" || err != nil { //https://github.com/adrg/xdg
		fmt.Println("Creating new config file")
		if err != nil {
			fmt.Println(err)
		}
		if overrides["configPath"] != "" {
			configFilePath = overrides["configPath"] + dbPath
		} else {
			configFilePath, err = xdg.ConfigFile(dbPath)
			if err != nil {
				log.Fatal("Error when creating file: ", err)
			}
		}
	}

	if err != nil {
		panic(err)
	}
	db, err := bolt.Open(configFilePath, 0600, &bolt.Options{Timeout: 30 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	a.db = db

}

func (a *App) DbClose() {
	defer a.db.Close()
}

func (a *App) DbStats() (string, error) {

	// Grab the current stats and diff them.
	stats := a.db.Stats()

	mJSON, err := json.Marshal(stats)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	mStr := string(mJSON)

	return mStr, nil
}

func (a *App) DbWrite(appId string, keys []string, values []string) (bool, error) { //TODO: how do I enforce that appid is the id of the callee?
	err := a.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(appId))
		if err != nil {
			return err
		}
		for i, key := range keys {
			err = b.Put([]byte(key), []byte(values[i]))
			if err != nil {
				return err
			}
		}
		return err
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) DbRead(appId string, keys []string) ([]string, error) {
	var values []string
	err := a.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(appId))
		if b == nil {
			return fmt.Errorf("bucket %q not found", appId)
		}
		for _, key := range keys {
			v := b.Get([]byte(key))
			if v == nil {
				return fmt.Errorf("key %q not found", key)
			}
			values = append(values, string(v))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return values, nil
}

func (a *App) DbDelete(appId string, keys []string) (bool, error) {
	err := a.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(appId))
		if b == nil {
			return fmt.Errorf("bucket %q not found", appId)
		}
		for _, key := range keys {
			err := b.Delete([]byte(key))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) DbDumpBackup() (bool, error) {
	dbp := a.db.Path() + "." + time.Now().Format("20060102") + ".bak"

	file, err := os.Create(dbp)
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	defer file.Close()

	err = a.db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(file)
		return err
	})
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	return true, nil
}

func (a *App) DbRestoreBackup(backupPath string) (bool, error) {
	err := a.db.Close()
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	err = os.Rename(a.db.Path(), a.db.Path()+"."+time.Now().Format("20060102")+".bak")
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	err = os.Rename(backupPath, a.db.Path())
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	db, err := bolt.Open(a.db.Path(), 0600, &bolt.Options{Timeout: 30 * time.Second})
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	a.db = db
	return true, nil
}

//TODO: create ORM like db functions
