package libzome

import (
	"context"
)

// App struct
type App struct {
	ctx          context.Context
	globalConfig ConfigObject
	peerRoom     *ChatRoom
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
