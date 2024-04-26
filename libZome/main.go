package libzome

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"github.com/jaketrock/zome/sharedInterfaces"
	"github.com/jaketrock/zome/zcrypto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
)

type ResBody struct {
	Code   int `json:"code,omitempty"`
	Status any `json:"status,omitempty"`
}

var zstate zcrypto.Zstate //TODO: bad idea?

//TODO: behavior:

// 1. initialize app to the topic
// 2. discover peers, use peerstore to find one where ID begins with your zome number(6char uuid)
// inherent security risk here, how to fix?
// 3. connect to that peer, exchange secure keys
// 4. send message only to that peer

func Initialize(topicName string, privateKey string) error {
	flag.Parse()
	ctx := context.Background()
	var pkeyUnmarshalled crypto.PrivKey
	var err error

	if privateKey == "" {
		pkeyUnmarshalled, err = generatePrivateKey()
		if err != nil {
			return err
		}
	} else {
		pkeyUnmarshalled, err = crypto.UnmarshalPrivateKey([]byte(privateKey))
		if err != nil {
			return err
		}
	}

	zstate = zcrypto.Zstate{
		Ctx: ctx,
	}
	err = zstate.InitP2P(pkeyUnmarshalled)
	if err != nil {
		return err
	}
	defer zstate.Close()

	err = zstate.AddCommTopic(topicName, func(msg *pubsub.Message) {
		fmt.Println("Received message: ", string(msg.Data))
	})
	if err != nil {
		return err
	}

	return nil
}

func generatePrivateKey() (crypto.PrivKey, error) {
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 1)
	if err != nil {
		return nil, err
	}
	return priv, nil
}

func GetSelfId(topic string) string {
	return zstate.SelfId(topic)
}

func ListPeers() []zcrypto.CleanPeer {
	return zstate.ConnectedPeersClean([]string{})
}

func ConnectServer(topic, peerId string) (ZomeController, error) {
	stream, err := zstate.DirectConnect(topic, peerId)
	if err != nil {
		return ZomeController{}, err
	}
	return ZomeController{socket: stream}, nil
}

func WaitForConnection(topicName, peerId string) (ZomeController, error) {
	for {
		select {
		case <-zstate.Ctx.Done():
			return ZomeController{}, fmt.Errorf("context done")
		default:
			peerList := ListPeers()
			//check if peer is in list
			pidFound := false
			for _, p := range peerList {
				if p.ID == peerId {
					pidFound = true
				}
			}
			if len(peerList) != 0 && pidFound {
				fmt.Println(peerList)
				return ConnectServer(topicName, peerId)
			}
			//loop sending info
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func genericRequest(ns network.Stream, routeName string, inputJson interface{}) (interface{}, error) {
	rqJson := sharedInterfaces.Request{
		Action: routeName,
		Data:   inputJson,
	}
	//json to bytes
	rqBytes, err := json.Marshal(rqJson)
	if err != nil {
		return nil, err
	}
	_, err = ns.Write(rqBytes)
	if err != nil {
		return nil, err
	}
	unmarshalledResponse := struct {
		Code   int         `json:"code"`
		Status interface{} `json:"status"`
	}{}
	var sResp []byte
	_, err = ns.Read(sResp)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(sResp, &unmarshalledResponse)
	if err != nil {
		return nil, err
	}
	if unmarshalledResponse.Code != 200 {
		if status, ok := unmarshalledResponse.Status.(map[string]interface{}); ok && status["Error"] != nil {
			return nil, fmt.Errorf("error %d: %v", unmarshalledResponse.Code, status["Error"])
		} else {
			return nil, fmt.Errorf("error %d: %v", unmarshalledResponse.Code, unmarshalledResponse.Status)
		}
	}
	return unmarshalledResponse.Status, nil
}

var errGenericType = fmt.Errorf("invalid response type")
