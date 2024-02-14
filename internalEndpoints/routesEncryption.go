package internalEndpoints

import (
	"zome/libzome"

	"github.com/gofiber/fiber/v2"
)

func (a *ZomeApi) routesEncryption(c *fiber.Ctx, data GenericRequestStruct) error {
	switch data.FunctionType {
	case "encrypt":
		{
			type encryptValue struct {
				cipherable string
			}
			subBody := encryptValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			ecResult := []libzome.MessagePod{}

			cipherableBytes := []byte(subBody.cipherable)
			err = a.zome.EcEncrypt(data.ApplicationTargetId, &cipherableBytes, func(input libzome.MessagePod) error {
				ecResult = append(ecResult, input)
				return nil
			})
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}
			return c.JSON(fiber.Map{
				"applicationTargetId": data.ApplicationTargetId,
				"requestId":           data.RequestId,
				"result":              ecResult,
			})
		}
	case "decrypt":
		{
			type decryptValue struct {
				decipherable []libzome.MessagePod
			}
			subBody := decryptValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			ecResult, err := a.zome.EcDecrypt(subBody.decipherable)
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}
			return c.JSON(fiber.Map{
				"applicationTargetId": data.ApplicationTargetId,
				"requestId":           data.RequestId,
				"result":              ecResult,
			})

		}
	case "sign":
		{
			type signValue struct {
				toSign string
			}
			subBody := signValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			ecResult, err := a.zome.EcSign([]byte(subBody.toSign))
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}
			return c.JSON(fiber.Map{
				"applicationTargetId": data.ApplicationTargetId,
				"requestId":           data.RequestId,
				"result":              ecResult,
			})
		}
	case "checksig":
		{
			type checksigValue struct {
				toCheck string
				sig     string
			}
			subBody := checksigValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			ecResult, err := a.zome.EcCheckSig([]byte(subBody.toCheck), subBody.sig)
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}
			return c.JSON(fiber.Map{
				"applicationTargetId": data.ApplicationTargetId,
				"requestId":           data.RequestId,
				"result":              ecResult,
			})
		}
	case "fileSign":
		{
			type fileSignValue struct {
				filePath string
			}
			subBody := fileSignValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			ecResult, err := a.zome.FsSignFile(data.ApplicationTargetId, subBody.filePath)
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}
			return c.JSON(fiber.Map{
				"applicationTargetId": data.ApplicationTargetId,
				"requestId":           data.RequestId,
				"result":              ecResult,
			})
		}
	case "fileCheckSig":
		{
			type fileCheckSigValue struct {
				filePath string
				sig      string
			}
			subBody := fileCheckSigValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			ecResult, err := a.zome.FsCheckSigFile(data.ApplicationTargetId, subBody.filePath, subBody.sig)
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}
			return c.JSON(fiber.Map{
				"applicationTargetId": data.ApplicationTargetId,
				"requestId":           data.RequestId,
				"result":              ecResult,
			})
		}
	default:
		{
			return c.Status(400).SendString("Bad Request Function Type")
		}
	}
}
