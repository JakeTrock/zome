package libzome

import (
	"context"

	bolt "go.etcd.io/bbolt"
)

// App struct
type App struct {
	ctx          context.Context
	cfgPath      string
	globalConfig ConfigObject
	Abilities    []string
	overrides    map[string]string

	PeerRoom *ChatRoom //TODO: eventually instead of one public channel and ungranular perms, we should use private map[string]*ChatRoom
	db       *bolt.DB
}

type ConfigObject struct {
	uuid           string
	poolId         string
	userName       string
	PubKey64       [1184]byte
	PrivKey64      [2400]byte
	knownKeypairs  map[string][1184]byte
	enabledPlugins []string //enabled plugins list of sha256 hashes
}

type ConfigPickled struct {
	Uuid           string
	PoolId         string
	UserName       string
	PubKey64       string
	PrivKey64      string
	KnownKeypairs  map[string]string
	EnabledPlugins []string
}
