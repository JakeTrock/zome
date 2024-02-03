package libzome

//https://github.com/libp2p/go-libp2p/tree/master/examples/chat-with-mdns

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// shutdown is called when the app is shutting down
func (a *App) Shutdown() {
	//todo: send save config signal
	a.terminateConnection()
}

//https://archive.org/details/youtube-l4Kijuav3ts
