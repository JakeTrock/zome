package internalEndpoints

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

func (a *ZomeApi) routesFs(c *fiber.Ctx, data GenericRequestStruct) error {
	switch data.FunctionType {
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
		fsResult, err := a.zome.FsDeleteFileOrFolder(data.ApplicationTargetId, subBody.paths, subBody.isFolder)
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
		fsResult, err := a.zome.FsMoveFileOrFolder(data.ApplicationTargetId, subBody.oldPath, subBody.newPath)
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
		fsResult, err := a.zome.FsCopyFileOrFolder(data.ApplicationTargetId, subBody.oldPath, subBody.newPath, subBody.isFolder)
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
		fsResult, err := a.zome.FsGetDirectoryListing(data.ApplicationTargetId, subBody.path, subBody.includeHash)
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
		fsResult, err := a.zome.FsCreateFolder(data.ApplicationTargetId, subBody.path)
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
		fsResult, err := a.zome.FsGetHash(data.ApplicationTargetId, subBody.path, subBody.isFolder)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.JSON(fiber.Map{
			"applicationTargetId": data.ApplicationTargetId,
			"requestId":           data.RequestId,
			"result":              fsResult,
		})

	case "write":
		form, err := c.MultipartForm()
		if err != nil {
			return err
		}
		// => *multipart.Form

		// Get all files from "documents" key:
		files := form.File["files"]
		// => []*multipart.FileHeader

		// Loop through files:
		for _, file := range files {
			fmt.Println(file.Filename, file.Size, file.Header["Content-Type"][0])
			// => "tutorial.pdf" 360641 "application/pdf"

			// Save the files to disk:
			a.zome.FsUploadFile(data.ApplicationTargetId, file)

			// Check for errors
			if err != nil {
				return err
			}
		}
	case "read":
		type readFile struct {
			path string
		}
		subBody := readFile{}
		err := c.BodyParser(&subBody)
		if err != nil {
			return c.Status(400).SendString("error parsing request")
		}
		file, err := a.zome.FsDownloadFile(data.ApplicationTargetId, subBody.path)
		if file == nil {
			return c.Status(404).SendString("File not found")
		}
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		return c.SendStream(file)

	default:
		return c.Status(400).SendString("Bad Request Function Type")
	}
	return c.Status(500).SendString("Unknown Error fs")
}
