package handlers

import (
	"github.com/seanime-app/seanime-server/internal/constants"
	"github.com/seanime-app/seanime-server/internal/entities"
)

type mediaEntryQuery struct {
	MediaId int `query:"mediaId" json:"mediaId"`
}

func HandleGetMediaEntry(c *RouteCtx) error {

	p := new(mediaEntryQuery)
	if err := c.Fiber.QueryParser(p); err != nil {
		return c.RespondWithError(err)
	}

	// Get all the local files
	lfs, err := getLocalFilesFromDB(c.App.Database)
	if err != nil {
		return c.RespondWithError(err)
	}

	// Get the user's anilist collection
	anilistCollection, err := c.App.GetAnilistCollection()
	if err != nil {
		return c.RespondWithError(err)
	}

	// Create a new media entry
	entry, err := entities.NewMediaEntry(&entities.NewMediaEntryOptions{
		MediaId:           p.MediaId,
		LocalFiles:        lfs,
		AnizipCache:       c.App.AnizipCache,
		AnilistCollection: anilistCollection,
		AnilistClient:     c.App.AnilistClient,
	})
	if err != nil {
		return c.RespondWithError(err)
	}

	// Fetch media details in the background and send them via websocket
	go func() {
		details, err := c.App.AnilistClient.MediaDetailsByID(c.Fiber.Context(), &p.MediaId)
		if err == nil {
			c.App.WSEventManager.SendEvent(constants.EventMediaDetails, details)
		}
	}()

	return c.RespondWithData(entry)
}
