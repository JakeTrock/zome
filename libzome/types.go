package libzome

import (
	"context"

	"go.dedis.ch/kyber/v3"
	bolt "go.etcd.io/bbolt"
)

// App struct
type App struct {
	ctx          context.Context
	cfgPath      string
	globalConfig ConfigObject
	Abilities    []string
	Overrides    map[string]string

	PeerRoom *PeerRoom
	db       *bolt.DB
}

type PeerState struct {
	key      kyber.Point
	approved bool
}

type ConfigObject struct {
	uuid           string
	poolId         string
	userName       string
	PubKeyHex      kyber.Point
	PrivKeyHex     kyber.Scalar
	knownKeypairs  map[string]PeerState
	enabledPlugins []string //enabled plugins list of sha256 hashes
}

type PeerStatePickled struct {
	key      string
	approved bool
}

type ConfigPickled struct {
	Uuid           string
	PoolId         string
	UserName       string
	PubKeyHex      string
	PrivKeyHex     string
	KnownKeypairs  map[string]PeerStatePickled
	EnabledPlugins []string
}
