package internalEndpoints

import "github.com/gofiber/fiber/v2"

func (a *ZomeApi) routesDb(c *fiber.Ctx, data GenericRequestStruct) error {
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
		dbResult, err := a.zome.DbRead(data.ApplicationTargetId, subBody.keys)
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
		dbResult, err := a.zome.DbDelete(data.ApplicationTargetId, subBody.keys)
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
		dbResult, err := a.zome.DbWrite(data.ApplicationTargetId, subBody.keys, subBody.values)
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
