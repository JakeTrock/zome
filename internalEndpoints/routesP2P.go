package internalEndpoints

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"time"
	"zome/libzome"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func sseAttach(c *fiber.Ctx, mChannel chan *libzome.PeerMessage, appId string) { //TODO: appid filtration logic should be inside libzome, also this shouldn't be just peermessage
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		peerRefreshTicker := time.NewTicker(time.Second)
		defer peerRefreshTicker.Stop()

		for {
			select {
			case m := <-mChannel:
				mJson, err := json.Marshal(m)
				mStr := string(mJson)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Fprintf(w, mStr)
				fmt.Println(mStr)

				err = w.Flush()
				if err != nil {
					// Refreshing page in web browser will establish a new
					// SSE connection, but only (the last) one is alive, so
					// dead connections must be closed here.
					fmt.Printf("Error while flushing: %v. Closing http connection.\n", err)

					break
				}
			}
		}

	}))
}

func (a *ZomeApi) routesP2P(c *fiber.Ctx, data GenericRequestStruct) error {
	switch data.FunctionType {
	case "send":
		type sendRequest struct { //TODO: allow streaming upload for large sends
			message string
		}
		subBody := sendRequest{}
		err := c.BodyParser(&subBody)
		if err != nil {
			return c.Status(400).SendString("error parsing request")
		}
		messageBytes := []byte(subBody.message)
		a.zome.P2PPushMessage(&messageBytes, data.RequestId, data.ApplicationTargetId)

		return c.JSON(fiber.Map{
			"applicationTargetId": data.ApplicationTargetId,
			"requestId":           data.RequestId,
		})
	case "attachReadSSE":
		sseAttach(c, a.zome.PeerRoom.Messages, data.ApplicationTargetId) //TODO: reassembly should occur before this, this should get a full dereffed obj
	case "getPeers":
		peers := a.zome.P2PGetPeers()
		return c.JSON(fiber.Map{
			"applicationTargetId": data.ApplicationTargetId,
			"requestId":           data.RequestId,
			"peers":               peers,
		})

	default:
		return c.Status(400).SendString("Bad Request Function Type")
	}
	return c.Status(500).SendString("Unknown Error p2p")
}
