package internalEndpoints

import (
	"fmt"
	libzome "zome/libzome"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

type GenericRequestStruct struct {
	RequestId           string `json:"requestId"`           //identifies the request microsession
	ApplicationTargetId string `json:"applicationTargetId"` //identifies the application requesting

	RequestType  string `json:"requestType"`  //identifies request permission container
	FunctionType string `json:"functionType"` //identifies the function to be called within container

	Data string `json:"data"` //request data
}

type ZomeAdminRequest struct {
	RequestType  string `json:"requestType"`  //identifies request permission container
	FunctionType string `json:"functionType"` //identifies the function to be called within container

	Data string `json:"data"` //request data
}

func InitEndpoints(zomeApp *libzome.App) {
	app := fiber.New(fiber.Config{
		StreamRequestBody: true,
	})

	zomeApi := NewApi(zomeApp, app)

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
	}))

	zomeApi.adminFunctions()

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
				return zomeApi.routesDb(c, data)
			}
		case "p2p":
			{
				return zomeApi.routesP2P(c, data)
			}
		case "encryption":
			{
				return zomeApi.routesEncryption(c, data)
			}
		case "fs":
			{
				return zomeApi.routesFs(c, data)
			}
		default:
			return c.Status(400).SendString("Bad Request Type")
		}
	})

	if err := app.Listen(":3000"); err != nil {
		fmt.Println(err)
	}
}
