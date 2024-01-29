package libzome

import (
	"context"
)

// App struct
type App struct {
	ctx          context.Context
	globalConfig ConfigObject
}

type ConfigObject struct {
	uuid           string
	userName       string
	PubKey64       [1184]byte
	PrivKey64      [2400]byte
	knownKeypairs  map[string][1184]byte
	enabledPlugins []string //enabled plugins list of sha256 hashes
}

type ConfigPickled struct {
	uuid           string
	userName       string
	PubKey64       string
	PrivKey64      string
	knownKeypairs  map[string]string
	enabledPlugins []string
}
