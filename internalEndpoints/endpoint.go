package internalEndpoints

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	libzome "zome/libzome"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/valyala/fasthttp"
)

type GenericRequestStruct struct {
	RequestId           string `json:"requestId"`           //identifies the request microsession
	ApplicationTargetId string `json:"applicationTargetId"` //identifies the application requesting

	RequestType  string `json:"requestType"`  //identifies request permission container
	FunctionType string `json:"functionType"` //identifies the function to be called within container

	Data string `json:"data"` //request data
}

func sseAttach(c *fiber.Ctx, mChannel chan *libzome.ChatMessage, appId string) { //TODO: appid filtration logic should be inside libzome, also this shouldn't be just chatmessage
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

func InitEndpoints(zomeApp *libzome.App) {
	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
	}))

	app.Get("/capabilities", func(c *fiber.Ctx) error { //TODO: get list of perms it can satisfy
		return c.SendString(fmt.Sprintf("[%v]", strings.Join(zomeApp.Abilities, ",")))
	})

	//TODO: create admin level route for getting db status, setting up isolation folders, etc

	app.Get("/capabilities", func(c *fiber.Ctx) error { //TODO: get list of perms it can satisfy
		return c.SendString(fmt.Sprintf("[%v]", strings.Join(zomeApp.Abilities, ",")))
	})

	app.Post("/zomeApi", func(c *fiber.Ctx) error {
		data := GenericRequestStruct{}
		err := c.BodyParser(&data)
		if err != nil {
			return c.Status(400).SendString("error parsing request")
		}
		switch data.RequestType {
		case "testing":
			{

				return c.JSON(fiber.Map{
					"applicationTargetId": data.ApplicationTargetId,
					"requestId":           data.RequestId,
					"result":              "success",
				})
			}
		case "database":
			{
				switch data.FunctionType {
				case "read":
					type readRequest struct {
						keys []string
					}
					subBody := readRequest{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					dbResult, err := zomeApp.DbRead(data.ApplicationTargetId, subBody.keys)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              dbResult,
					})
				case "delete":
					type deleteRequest struct {
						keys []string
					}
					subBody := deleteRequest{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					dbResult, err := zomeApp.DbDelete(data.ApplicationTargetId, subBody.keys)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              dbResult,
					})
				case "write":
					type writeRequest struct {
						keys   []string
						values []string
					}
					subBody := writeRequest{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					dbResult, err := zomeApp.DbWrite(data.ApplicationTargetId, subBody.keys, subBody.values)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              dbResult,
					})
				default:
					return c.Status(400).SendString("Bad Request Function Type")
				}
			}
		case "p2p":
			{
				switch data.FunctionType {
				case "send":
					type readRequest struct {
						message string
					}
					subBody := readRequest{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					zomeApp.P2PPushMessage(subBody.message, data.ApplicationTargetId)

					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
					})
				case "attachReadSSE":
					sseAttach(c, zomeApp.PeerRoom.Messages, data.ApplicationTargetId)
				case "getPeers":
					peers := zomeApp.P2PGetPeers()
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"peers":               peers,
					})

				default:
					return c.Status(400).SendString("Bad Request Function Type")
				}
			}
		case "encryption":
			{
				switch data.FunctionType {
				case "encrypt":
					return c.Status(500).SendString("Not yet implemented") //TODO: implementme
				case "decrypt":
					return c.Status(500).SendString("Not yet implemented") //TODO: implementme
				case "sign":
					return c.Status(500).SendString("Not yet implemented") //TODO: implementme
				case "checksig":
					return c.Status(500).SendString("Not yet implemented") //TODO: implementme
				default:
					return c.Status(400).SendString("Bad Request Function Type")
				}
			}
		case "fs":
			{
				switch data.FunctionType {
				case "read":
					type copyFiles struct {
						oldPath  string
						newPath  string
						isFolder bool
					}
					subBody := copyFiles{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					fsResult, err := zomeApp.FsCopyFileOrFolder(data.ApplicationTargetId, subBody.oldPath, subBody.newPath, subBody.isFolder)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              fsResult,
					})
				case "delete":
					type deleteFiles struct {
						paths    []string
						isFolder []bool
					}
					subBody := deleteFiles{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					fsResult, err := zomeApp.FsDeleteFileOrFolder(data.ApplicationTargetId, subBody.paths, subBody.isFolder)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              fsResult,
					})
				case "move":
					type moveFiles struct {
						oldPath string
						newPath string
					}
					subBody := moveFiles{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					fsResult, err := zomeApp.FsMoveFileOrFolder(data.ApplicationTargetId, subBody.oldPath, subBody.newPath)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              fsResult,
					})
				case "copy":
					type copyFiles struct {
						oldPath  string
						newPath  string
						isFolder bool
					}
					subBody := copyFiles{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					fsResult, err := zomeApp.FsCopyFileOrFolder(data.ApplicationTargetId, subBody.oldPath, subBody.newPath, subBody.isFolder)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              fsResult,
					})
				case "list":
					type listFiles struct {
						path        string
						includeHash bool
					}
					subBody := listFiles{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					fsResult, err := zomeApp.FsGetDirectoryListing(data.ApplicationTargetId, subBody.path, subBody.includeHash)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              fsResult,
					})
				case "createFolder":
					type createFolder struct {
						path string
					}
					subBody := createFolder{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					fsResult, err := zomeApp.FsCreateFolder(data.ApplicationTargetId, subBody.path)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              fsResult,
					})
				case "getHash":
					type getHash struct {
						path     string
						isFolder bool
					}
					subBody := getHash{}
					err := c.BodyParser(&subBody)
					if err != nil {
						return c.Status(400).SendString("error parsing request")
					}
					fsResult, err := zomeApp.FsGetHash(data.ApplicationTargetId, subBody.path, subBody.isFolder)
					if err != nil {
						return c.Status(500).SendString(err.Error())
					}
					return c.JSON(fiber.Map{
						"applicationTargetId": data.ApplicationTargetId,
						"requestId":           data.RequestId,
						"result":              fsResult,
					})

				case "write/write":
					return c.Status(500).SendString("Not yet implemented") //TODO: implementme w/ streemz
				default:
					return c.Status(400).SendString("Bad Request Function Type")
				}
			}
		default:
			return c.Status(400).SendString("Bad Request Type")
		}
		return c.Status(500).SendString("Unknown Error")
	})

	app.Get("/streamChannel", func(c *fiber.Ctx) error {
		return c.SendString("streamChannel") //TODO: implement ftp like upload/download channel
	})

	if err := app.Listen(":3000"); err != nil {
		fmt.Println(err)
	}
}
