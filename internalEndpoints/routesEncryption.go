package internalEndpoints

import (
	"bufio"
	"bytes"

	"github.com/gofiber/fiber/v2"
)

func (a *ZomeApi) routesEncryption(c *fiber.Ctx, data GenericRequestStruct) error {
	switch data.FunctionType {
	case "encrypt":
		{
			type encryptValue struct {
				cipherable string
				toUUID     string
			}
			subBody := encryptValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}

			cipherableBytes := []byte(subBody.cipherable)
			emptyBuffer := bytes.NewBuffer([]byte{})
			emptyWriter := bufio.NewWriter(emptyBuffer)
			messageReader := bufio.NewReadWriter(bufio.NewReader(bytes.NewReader(cipherableBytes)), emptyWriter)
			ecLength := len(cipherableBytes)
			err = a.zome.EcEncrypt(subBody.toUUID, ecLength, *messageReader)
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}

			return c.JSON(fiber.Map{
				"applicationTargetId": data.ApplicationTargetId,
				"requestId":           data.RequestId,
				"result":              emptyBuffer.Bytes(),
			})
		}
	case "decrypt":
		{
			type decryptValue struct {
				decipherable []byte
			}
			subBody := decryptValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			cipherableBytes := []byte(subBody.decipherable)
			emptyBuffer := bytes.NewBuffer([]byte{})
			emptyWriter := bufio.NewWriter(emptyBuffer)
			messageReader := bufio.NewReadWriter(bufio.NewReader(bytes.NewReader(cipherableBytes)), emptyWriter)
			err = a.zome.EcDecrypt(*messageReader)
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}
			return c.JSON(fiber.Map{
				"applicationTargetId": data.ApplicationTargetId,
				"requestId":           data.RequestId,
				"result":              emptyBuffer.Bytes(),
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
				peerId  string
				sig     string
			}
			subBody := checksigValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			ecResult, err := a.zome.EcCheckSig([]byte(subBody.toCheck), subBody.peerId, subBody.sig)
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
				peerId   string
				sig      string
			}
			subBody := fileCheckSigValue{}
			err := c.BodyParser(&subBody)
			if err != nil {
				return c.Status(400).SendString("error parsing request")
			}
			ecResult, err := a.zome.FsCheckSigFile(data.ApplicationTargetId, subBody.filePath, subBody.sig, subBody.peerId)
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
