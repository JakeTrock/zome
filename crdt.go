package main

import (
	"fmt"

	ds "github.com/ipfs/go-datastore"
	"github.com/jaketrock/zome/zcrypto"
)

// allow/disallow adding values to a domain without said value there yet(meaning no acl would exist)
func (a *App) globalWriteAbstract(origin string, key string) (bool, error) {
	selfOrigin := origin + "]-" + key
	value, err := a.secureInternalKeyGet(selfOrigin)
	if err != nil {
		if err != ds.ErrNotFound {
			return false, err
		} else {
			//repair db to secure state
			err = a.secureInternalKeyAdd(selfOrigin, []byte{0})
			if err != nil {
				return false, err
			}
			return false, nil
		}
	}
	if value[0] == 1 {
		return true, nil
	}
	return false, nil
}

func (a *App) secureAddLoop(addValues map[string]string, ACL, origin, selfOrigin string) (map[string]bool, error) {

	success := make(map[string]bool, len(addValues))

	acl, err := zcrypto.SanitizeACL(ACL)
	if err != nil {
		a.Logger.Error(err)
		return nil, err
	}

	var globalWrite = false
	if origin != selfOrigin {
		globalWrite, err = a.globalWriteAbstract(origin, "GW")
		if err != nil {
			a.Logger.Error(err)
			return nil, err
		}
	} else {
		globalWrite = true
	}

	for k, v := range addValues {
		// check value exists
		didSucceed, err := a.secureAdd(k, v, acl, origin, selfOrigin, globalWrite)
		if err != nil {
			a.Logger.Error(err)
			success[k] = false
		}
		success[k] = didSucceed
	}
	return success, nil
}

func (a *App) secureGetLoop(getValues []string, origin string, selfOrigin string) (map[string]string, error) {
	success := make(map[string]string, len(getValues))

	for _, k := range getValues {
		// Retrieve the value from the store
		decryptedValue, err := a.secureGet(k, origin, selfOrigin)
		if err != nil {
			a.Logger.Error(err)
			success[k] = ""
		}
		success[k] = decryptedValue
	}
	return success, nil
}

func (a *App) secureAdd(key, valueText, acl, origin, selfOrigin string, globalWrite bool) (bool, error) {
	priorValue, err := a.store.Get(a.ctx, ds.NewKey(origin))
	if err != nil {
		if err != ds.ErrNotFound {
			return false, err
		} else if !globalWrite && origin != selfOrigin { // only continue if the global write is enabled
			return false, fmt.Errorf("global write not enabled")
		}
	} else if !zcrypto.CheckACL(string(priorValue[:2]), "2", selfOrigin, origin) {
		return false, fmt.Errorf("acl failure")
	}

	var encBytes []byte
	// Encrypt the value with the private key

	encryptedValue, err := zcrypto.AesGCMEncrypt(a.dbCryptKey, []byte(valueText))
	if err != nil {
		a.Logger.Error(err)
		return false, err
	}
	encBytes = append([]byte(acl), encryptedValue...)
	err = a.store.Put(a.ctx, ds.NewKey(origin+"-"+key), encBytes)
	if err != nil {
		a.Logger.Error(err)
		return false, err
	}
	return true, nil
}

func (a *App) secureGet(key, origin, originSelf string) (string, error) {
	// Retrieve the value from the store
	value, err := a.store.Get(a.ctx, ds.NewKey(origin+"-"+key))
	if err != nil {
		if err == ds.ErrNotFound {
			return "", nil
		}
		a.Logger.Error(err)
		return "", err
	}
	//check ACL
	if !zcrypto.CheckACL(string(value[:2]), "1", originSelf, origin) {
		return "", fmt.Errorf("acl failure")
	}
	// Decrypt the value with the private key
	decryptedValue, err := zcrypto.AesGCMDecrypt(a.dbCryptKey, value[2:]) //chop acl off
	if err != nil {
		a.Logger.Error(err)
		return "", err
	}

	return string(decryptedValue), nil
}

// securely add key for internal pool use only
func (a *App) secureInternalKeyAdd(key string, value []byte) error {
	// Encrypt the value with the private key
	encBytes, err := zcrypto.AesGCMEncrypt(a.dbCryptKey, value)
	if err != nil {
		a.Logger.Error(err)
		return err
	}
	err = a.store.Put(a.ctx, ds.NewKey(key), encBytes)
	if err != nil {
		a.Logger.Error(err)
		return err
	}
	return nil
}

// securely get key for internal pool use only
func (a *App) secureInternalKeyGet(key string) ([]byte, error) {
	// Retrieve the value from the store
	value, err := a.store.Get(a.ctx, ds.NewKey(key))
	if err != nil {
		if err == ds.ErrNotFound {
			return nil, nil
		}
		a.Logger.Error(err)
		return nil, err
	}
	// Decrypt the value with the private key
	decryptedValue, err := zcrypto.AesGCMDecrypt(a.dbCryptKey, value[2:]) //chop acl off
	if err != nil {
		a.Logger.Error(err)
		return nil, err
	}

	return decryptedValue, nil
}
