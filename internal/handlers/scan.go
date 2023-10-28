package handlers

import (
	"errors"
	"github.com/seanime-app/seanime-server/internal/scanner"
)

type ScanRequestBody struct {
	Username string `json:"username"`
	Enhanced bool   `json:"enhanced"`
}

func HandleScanLocalFiles(c *RouteCtx) error {

	c.AcceptJSON()

	token := c.GetAnilistToken()

	// Retrieve the user's library path
	libraryPath, err := c.App.Database.GetLibraryPath()
	if err != nil {
		return c.RespondWithError(err)
	}

	// Body
	body := new(ScanRequestBody)
	if err := c.Fiber.BodyParser(body); err != nil {
		return c.RespondWithError(err)
	}

	if len(body.Username) == 0 {
		return c.RespondWithError(errors.New("'username' is required"))
	}

	sc := scanner.Scanner{
		Token:         token,
		DirPath:       libraryPath,
		Username:      body.Username,
		Enhanced:      body.Enhanced,
		AnilistClient: c.App.AnilistClient,
		Logger:        c.App.Logger,
		DB:            c.App.Database,
	}

	localFiles, err := sc.Scan()
	if err != nil {
		return c.RespondWithError(err)
	}

	return c.RespondWithData(localFiles)

}
