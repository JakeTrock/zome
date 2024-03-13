package main

import ds "github.com/ipfs/go-datastore"

func (a *App) globalWriteAbstract(origin string) (bool, error) {
	selfOrigin := origin + "]-GW"
	value, err := a.store.Get(a.ctx, ds.NewKey(selfOrigin))
	if err != nil {
		if err != ds.ErrNotFound {
			return false, err
		} else {
			//repair db to secure state
			err = a.store.Put(a.ctx, ds.NewKey(selfOrigin+"]-GW"), []byte{0})
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

func (a *App) secureAddLoop(addValues map[string]string, ACL string, origin string, selfOrigin string) (map[string]bool, error) {

	success := make(map[string]bool, len(addValues))

	acl, err := sanitizeACL(ACL)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	var globalWrite = false
	if origin != selfOrigin {
		globalWrite, err = a.globalWriteAbstract(origin)
		if err != nil {
			logger.Error(err)
			return nil, err
		}
	} else {
		globalWrite = true
	}

	for k, v := range addValues {
		origin := origin + "-" + k
		// check value exists
		didSucceed, err := a.secureAdd(origin, v, acl, origin, globalWrite)
		if err != nil {
			logger.Error(err)
			success[k] = false
		}
		success[k] = didSucceed
	}
	return success, nil
}

func (a *App) secureGetLoop(getValues []string, origin string, selfOrigin string) (map[string]string, error) {
	success := make(map[string]string, len(getValues))

	for _, k := range getValues {
		origin := origin + "-" + k
		// Retrieve the value from the store
		decryptedValue, err := a.secureGet(origin, selfOrigin)
		if err != nil {
			logger.Error(err)
			success[k] = ""
		}
		success[k] = decryptedValue
	}
	return success, nil
}

func (a *App) secureAdd(origin string, valueText string, acl string, selfOrigin string, globalWrite bool) (bool, error) {
	priorValue, err := a.store.Get(a.ctx, ds.NewKey(origin))
	if err != nil {
		if err != ds.ErrNotFound {
			return false, err
		} else if !globalWrite { // only continue if the global write is enabled
			return false, nil
		}
	} else if checkACL(string(priorValue[:2]), "2", selfOrigin, origin) {
		return false, nil
	}

	var encBytes []byte
	// Encrypt the value with the private key
	encryptedValue, err := AesGCMEncrypt(a.dbCryptKey, []byte(valueText))
	if err != nil {
		logger.Error(err)
		return false, err
	}
	encBytes = append([]byte(acl), encryptedValue...)
	err = a.store.Put(a.ctx, ds.NewKey(origin), encBytes)
	if err != nil {
		logger.Error(err)
		return false, err
	}
	return true, nil
}

func (a *App) secureGet(origin string, originSelf string) (string, error) {
	// Retrieve the value from the store
	value, err := a.store.Get(a.ctx, ds.NewKey(origin))
	if err != nil {
		logger.Error(err)
		return "", err

	}
	//check ACL
	if checkACL(string(value[:2]), "1", originSelf, origin) {
		return "", nil
	}
	// Decrypt the value with the private key
	decryptedValue, err := AesGCMDecrypt(a.dbCryptKey, value[2:]) //chop acl off
	if err != nil {
		logger.Error(err)
		return "", err
	}

	return string(decryptedValue), nil
}
