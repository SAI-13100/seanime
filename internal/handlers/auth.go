package handlers

import (
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/seanime-app/seanime-server/internal/models"
	"time"
)

func anilistTokenMiddleware(c *fiber.Ctx) error {

	token := c.Cookies("anilistToken", "")

	if len(token) == 0 {
		return c.Status(fiber.StatusMethodNotAllowed).JSON(NewErrorResponse(errors.New("missing AniList token")))
	}

	return c.Next()

}

type AuthRequestBody struct {
	Token string
}

func HandleLogin(c *RouteCtx) error {

	c.Fiber.Accepts("application/json")

	// Body data
	body := new(AuthRequestBody)

	if err := c.Fiber.BodyParser(body); err != nil {
		return c.Fiber.JSON(NewErrorResponse(err))
	}

	// Re-init the client, this time by passing the JWT token
	c.App.UpdateAnilistClientToken(body.Token)

	// Get viewer data from AniList
	getViewer, err := c.App.AnilistClient.GetViewer(context.Background())
	if err != nil {
		c.App.Logger.Error().Msg("Could not authenticate to AniList")
		return c.RespondWithError(err)
	}

	if len(getViewer.Viewer.Name) == 0 {
		return c.RespondWithError(errors.New("could not find user"))
	}

	_, err = c.App.Database.UpsertAccount(&models.Account{
		BaseModel: models.BaseModel{
			ID:        1,
			UpdatedAt: time.Now(),
		},
		Username: getViewer.Viewer.Name,
		Token:    body.Token,
	})

	if err != nil {
		return c.RespondWithError(err)
	}

	c.App.Logger.Info().Msg("Authenticated to AniList as " + getViewer.Viewer.Name)

	return c.RespondWithData(getViewer.Viewer)

}
