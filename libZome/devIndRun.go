package libzome

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a topic as a command-line argument.")
		return
	}

	topic := os.Args[1]
	fmt.Println("Topic:", topic)
	libZome := &Zstate{}
	libZome.Initialize(topic)
	peerList := libZome.ListPeers(topic)
	if len(peerList) == 0 {
		fmt.Println("no peers found") //TODO: not mating?
	}
	var exists bool
	servConn, exists := libZome.ConnectServer(topic, peerList[0], ControlOp).(ZomeController)
	if !exists {
		fmt.Println("no connection found")
	}
	spid := libZome.SelfId(topic)
	fmt.Println(spid)
	gwres, err := servConn.DbGetGlobalWrite()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("gwr: ", gwres)
}
