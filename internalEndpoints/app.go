package internalEndpoints

import (
	"zome/libzome"

	"github.com/gofiber/fiber/v2"
)

type ZomeApi struct {
	zome  *libzome.App
	fiber *fiber.App
}

func NewApi(zome *libzome.App, fiber *fiber.App) *ZomeApi {
	return &ZomeApi{
		zome:  zome,
		fiber: fiber,
	}
}
