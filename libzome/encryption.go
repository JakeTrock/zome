package libzome

import (
	"fmt"

	"github.com/labstack/gommon/log"
	kyberk2so "github.com/symbolicsoft/kyber-k2so"
)

func (a *App) Encrypt(uuid string) ([1088]byte, error) { //https://github.com/SymbolicSoft/kyber-k2so
	//check if we have a keypair for this uuid
	knownKey, ok := a.globalConfig.knownKeypairs[uuid]
	if ok {
		ciphertext, _, err := kyberk2so.KemEncrypt768(knownKey)
		if err != nil {
			log.Fatal(err)
		}
		return ciphertext, nil
	}
	//throw error
	return [1088]byte{}, fmt.Errorf("no keypair found for uuid %s", uuid)
}

func (a *App) Decrypt(ciphertext [1088]byte) ([32]byte, error) {
	privateKey := a.globalConfig.PrivKey64
	ssB, error := kyberk2so.KemDecrypt768(ciphertext, privateKey)
	if error != nil {
		fmt.Println("error decrypting ciphertext")
		return [32]byte{}, fmt.Errorf("error decrypting ciphertext")
	}
	return ssB, nil
}
