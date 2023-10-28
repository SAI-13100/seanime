package anilist

import (
	"context"
	"errors"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	"github.com/seanime-app/seanime-server/internal/limiter"
)

func (c *Client) AddMediaToPlanning(mIds []int, rateLimiter *limiter.Limiter, logger *zerolog.Logger) error {
	if len(mIds) == 0 {
		logger.Debug().Msg("anilist: no media added to planning list")
		return nil
	}
	if rateLimiter == nil {
		return errors.New("anilist: no rate limiter provided")
	}

	status := MediaListStatusPlanning

	lo.ForEach(mIds, func(id int, index int) {
		rateLimiter.Wait()
		_, err := c.UpdateEntry(
			context.Background(),
			&id,
			&status,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		)
		if err != nil {
			logger.Error().Msg("anilist: An error  occurred while adding media to plannig list: " + err.Error())
		}
	})

	logger.Debug().Any("count", len(mIds)).Msg("anilist: Media added to planning list")

	return nil
}
