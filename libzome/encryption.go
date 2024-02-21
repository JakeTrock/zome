package libzome

import (
	"bufio"
	"encoding/hex"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
)

const keyType = crypto.Ed25519

func newKp() (crypto.PrivKey, crypto.PubKey, error) {
	privkey, _, err := crypto.GenerateKeyPair(keyType, 2048)
	if err != nil {
		return nil, nil, err
	}
	return privkey, privkey.GetPublic(), nil
}

func kpToString(privkey crypto.PrivKey, pubkey crypto.PubKey) (string, string, error) {
	rawPriv, err := crypto.MarshalPrivateKey(privkey)
	if err != nil {
		return "", "", err
	}
	rawPub, err := crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return "", "", err
	}

	return crypto.ConfigEncodeKey(rawPriv), crypto.ConfigEncodeKey(rawPub), nil
}

func stringToKp(privKey64 string, pubKey64 string) (crypto.PrivKey, crypto.PubKey, error) {
	privkey, err := crypto.ConfigDecodeKey(privKey64)
	if err != nil {
		return nil, nil, fmt.Errorf("pvdc: " + err.Error())
	}
	privObj, err := crypto.UnmarshalPrivateKey(privkey)
	if err != nil {
		return nil, nil, fmt.Errorf("pvkum: " + err.Error())
	}
	pubkey, err := crypto.ConfigDecodeKey(pubKey64)
	if err != nil {
		return nil, nil, fmt.Errorf("pbdc: " + err.Error())
	}
	pubObj, err := crypto.UnmarshalPublicKey(pubkey)
	if err != nil {
		return nil, nil, fmt.Errorf("pbkum: " + err.Error())
	}

	return privObj, pubObj, err
}

func pickleKnownKeypairs(knownKeypairs map[string]PeerState) (map[string]PeerStatePickled, error) {
	pickled := make(map[string]PeerStatePickled)
	for k, v := range knownKeypairs {
		rawKey, err := v.key.Raw()
		if err != nil {
			return nil, err
		}
		pickled[k] = PeerStatePickled{crypto.ConfigEncodeKey(rawKey), v.approved}
	}
	return pickled, nil
}

func unpickleKnownKeypairs(knownKeypairs map[string]PeerStatePickled) (map[string]PeerState, error) {
	unpickled := make(map[string]PeerState)
	for k, v := range knownKeypairs {
		key, err := crypto.ConfigDecodeKey(v.key)
		if err != nil {
			return nil, err
		}
		umkey, err := crypto.UnmarshalEd25519PublicKey(key)
		if err != nil {
			return nil, err
		}
		unpickled[k] = PeerState{umkey, v.approved}
	}
	return unpickled, nil
}

func (a *App) EcEncrypt(toUUID string, totalLen int, readWriter bufio.ReadWriter) error {
	//check if we have a keypair for this uuid
	peerState, ok := a.globalConfig.knownKeypairs[toUUID]
	if !ok {
		return fmt.Errorf("no keypair found for uuid")
	}
	if !peerState.approved {
		return fmt.Errorf("keypair not approved")
	}
	// Encrypt the message using the public key
	//TODO: not yet implemented
	return fmt.Errorf("not yet implemented")

}

func (a *App) EcDecrypt(readWriter bufio.ReadWriter) error {
	//TODO: not yet implemented
	return fmt.Errorf("not yet implemented")
}

func (a *App) EcSign(toSign []byte) (string, error) { //TODO: switch to streamed io
	sig, err := a.globalConfig.PrivKey.Sign(toSign)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig), nil
}

func (a *App) EcCheckSig(toCheck []byte, peerId string, sig string) (bool, error) {
	peerState, ok := a.globalConfig.knownKeypairs[peerId]
	if !ok {
		return false, fmt.Errorf("no keypair found for uuid")
	}
	if !peerState.approved {
		return false, fmt.Errorf("keypair not approved")
	}
	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		return false, err
	}
	return peerState.key.Verify(toCheck, sigBytes)
}
