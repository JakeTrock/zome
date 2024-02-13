package internalEndpoints

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (a *ZomeApi) adminFunctions() {
	a.fiber.Get("/zomeAdmin", func(c *fiber.Ctx) error {
		data := ZomeAdminRequest{}
		err := c.BodyParser(&data)
		if err != nil {
			return c.Status(400).SendString("error parsing request")
		}
		switch data.RequestType {
		case "root":
			{
				switch data.FunctionType {
				case "echo":
					{
						return c.JSON(fiber.Map{
							"FunctionType": data.FunctionType,
							"RequestType":  data.RequestType,
							"Data":         data.Data,
						})
					}
				case "capabilities":
					{
						return c.SendString(fmt.Sprintf("[%v]", strings.Join(a.zome.Abilities, ",")))
					}
				}
			}
		case "database":
			{
				switch data.FunctionType {
				case "dbinfo":
					{
						dbInfo, err := a.zome.DbStats()
						if err != nil {
							return c.Status(500).SendString(err.Error())
						}
						return c.JSON(fiber.Map{
							"data": dbInfo,
						})
					}
				}
			}
		default:
			return c.Status(400).SendString("Bad Request Type")
		}
		return c.Status(500).SendString("Unknown Error")

	})
}
