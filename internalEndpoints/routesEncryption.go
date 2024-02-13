package internalEndpoints

import "github.com/gofiber/fiber/v2"

func (a *ZomeApi) routesEncryption(c *fiber.Ctx, data GenericRequestStruct) error {
	switch data.FunctionType {
	case "encrypt":
		{
			return c.Status(500).SendString("Not yet implemented") //TODO: implementme
		}
	case "decrypt":
		{
			return c.Status(500).SendString("Not yet implemented") //TODO: implementme
		}
	case "sign":
		{
			return c.Status(500).SendString("Not yet implemented") //TODO: implementme
		}
	case "checksig":
		{
			return c.Status(500).SendString("Not yet implemented") //TODO: implementme
		}
	default:
		{
			return c.Status(400).SendString("Bad Request Function Type")
		}
	}
	return c.Status(500).SendString("Unknown Error encryption")
}
