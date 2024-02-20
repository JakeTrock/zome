package libzome

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"

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
	inputCh  chan PeerMessagePre

	ctx   context.Context
	ps    *pubsub.PubSub
	topic *pubsub.Topic
	sub   *pubsub.Subscription

	roomName string
	self     peer.ID
	nick     string
}

type PeerMessagePre struct {
	Message   []byte
	AppId     string
	RequestId string
}

// PeerMessage gets converted to/from JSON and sent in the body of pubsub messages.
type PeerMessage struct {
	Message   string
	SenderID  string
	RequestId string
	AppId     string
}

// JoinPeerRoom tries to subscribe to the PubSub topic for the room name, returning
// a PeerRoom on success.
func JoinPeerRoom(ctx context.Context, ps *pubsub.PubSub, nickname string, roomName string, peerId peer.ID) (*PeerRoom, error) {
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

	cr := &PeerRoom{
		ctx:      ctx,
		ps:       ps,
		topic:    topic,
		sub:      sub,
		self:     peerId,
		nick:     nickname,
		roomName: roomName,
		Messages: make(chan *PeerMessage, PeerRoomBufSize),
		inputCh:  make(chan PeerMessagePre, 512), //TODO: may need to bump this, depending on how encryption chunks it all
	}

	// start reading messages from the subscription in a loop
	go cr.readLoop()
	return cr, nil
}

// Publish sends a message to the pubsub topic.
func (cr *PeerRoom) Publish(pmsg PeerMessagePre) error {
	// chunk message into 128k chunks
	if len(pmsg.Message) > 128000 {
		return fmt.Errorf("Message too large")
	}

	m := PeerMessage{
		Message:   string(base64.StdEncoding.EncodeToString(pmsg.Message)),
		SenderID:  cr.self.String(),
		RequestId: pmsg.RequestId,
		AppId:     pmsg.AppId,
	}
	msgBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}
	//send message individually to each peer
	return cr.topic.Publish(cr.ctx, msgBytes)
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
