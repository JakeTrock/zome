package libzome

import (
	"context"
	"encoding/json"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/group/edwards25519"
	encoder "go.dedis.ch/kyber/v3/util/encoding"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// PeerRoomBufSize is the number of incoming messages to buffer for each topic.
const PeerRoomBufSize = 128

// PeerRoom represents a subscription to a single PubSub topic. Messages
// can be published to the topic with PeerRoom.Publish, and received
// messages are pushed to the Messages channel.
type PeerRoom struct {
	// Messages is a channel of messages received from other peers in the peer room
	Messages chan *PeerMessage
	inputCh  chan CipherMessagePre

	ctx   context.Context
	ps    *pubsub.PubSub
	topic *pubsub.Topic
	sub   *pubsub.Subscription

	roomName string
	self     peer.ID
	nick     string
}

type PeerMessagePre struct {
	Message *[]byte
	AppId   string
}

// PeerMessage gets converted to/from JSON and sent in the body of pubsub messages.
type PeerMessage struct {
	Message    string
	SenderID   string
	AppId      string
	SenderNick string
}

type CipherMessagePre struct {
	Message   *[]byte
	RequestId string
	AppId     string
}

type CipherMessage struct {
	Message    MessagePod
	RequestId  string //used to reassemble ciphertext
	SenderID   string
	AppId      string
	SenderNick string
}

// JoinPeerRoom tries to subscribe to the PubSub topic for the room name, returning
// a PeerRoom on success.
func JoinPeerRoom(ctx context.Context, ps *pubsub.PubSub, nickname string, roomName string, pubKey kyber.Point) (*PeerRoom, error) {
	// join the pubsub topic
	topic, err := ps.Join(topicName(roomName))
	if err != nil {
		return nil, err
	}

	// and subscribe to it
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}
	suite := edwards25519.NewBlakeSHA256Ed25519()
	pubKeyStr, err := encoder.PointToStringHex(suite, pubKey)
	if err != nil {
		return nil, err
	}

	cr := &PeerRoom{
		ctx:      ctx,
		ps:       ps,
		topic:    topic,
		sub:      sub,
		self:     peer.ID(pubKeyStr),
		nick:     nickname,
		roomName: roomName,
		Messages: make(chan *PeerMessage, PeerRoomBufSize),
		inputCh:  make(chan CipherMessagePre, 512), //TODO: may need to bump this, depending on how encryption chunks it all
	}

	// start reading messages from the subscription in a loop
	go cr.readLoop()
	// cr.SharePubkey(pubKey)//TODO: may not be needed if pubkey id used
	return cr, nil
}

type PeerPubKey struct {
	PubKey     kyber.Point
	SenderID   string
	AppId      string
	SenderNick string
}

// func (cr *PeerRoom) SharePubkey(pmsg kyber.Point) error {//TODO: may not be needed if pubkey id used
// 	m := PeerPubKey{
// 		SenderID:   cr.self.String(),
// 		SenderNick: cr.nick,
// 		AppId:      "pubkey",
// 		PubKey:     pmsg,
// 	}
// 	msgBytes, err := json.Marshal(m)
// 	if err != nil {
// 		return err
// 	}
// 	return cr.topic.Publish(cr.ctx, msgBytes)
// }

// Publish sends a message to the pubsub topic.
func (cr *PeerRoom) Publish(pmsg PeerMessagePre) error {
	m := PeerMessage{
		Message:    string(*pmsg.Message),
		SenderID:   cr.self.String(),
		SenderNick: cr.nick,
		AppId:      pmsg.AppId,
	}
	msgBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}
	//send message individually to each peer
	return cr.topic.Publish(cr.ctx, msgBytes)
}
func (cr *PeerRoom) PublishCrypt(pmsg CipherMessagePre, encryptFns []func(input *[]byte, mHook func(input MessagePod) error) error) error {

	//send message individually to each peer
	for _, encryptBytes := range encryptFns { //TODO: multithread this
		err := encryptBytes(pmsg.Message, func(input MessagePod) error {
			m := CipherMessage{
				Message:    input,
				RequestId:  pmsg.RequestId, //used for reassembly
				SenderID:   cr.self.String(),
				SenderNick: cr.nick,
				AppId:      pmsg.AppId,
			}
			msgBytes, err := json.Marshal(m)
			if err != nil {
				return err
			}
			return cr.topic.Publish(cr.ctx, msgBytes) //TODO: send to individual peers? don't want to flood the ether
		})
		if err != nil {
			return err
		}
	}
	return nil //TODO: return error?
}

func (cr *PeerRoom) ListPeers() []peer.ID {
	return cr.ps.ListPeers(topicName(cr.roomName))
}

func (cr *PeerRoom) GetRoomName() string {
	return cr.roomName
}

func (cr *PeerRoom) GetRoomNick() string {
	return cr.nick
}

func (cr *PeerRoom) Leave() {
	cr.sub.Cancel()
}

// readLoop pulls messages from the pubsub topic and pushes them onto the Messages channel.
func (cr *PeerRoom) readLoop() {
	for {
		msg, err := cr.sub.Next(cr.ctx)
		if err != nil {
			close(cr.Messages)
			return
		}
		// only forward messages delivered by others
		if msg.ReceivedFrom == cr.self {
			continue
		}
		cm := new(PeerMessage)
		err = json.Unmarshal(msg.Data, cm)
		if err != nil {
			continue
		}
		// send valid messages onto the Messages channel
		cr.Messages <- cm
	}
}

func topicName(roomName string) string {
	return "peer-room:" + roomName
}
