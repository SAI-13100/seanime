package handlers

import (
	"errors"
	"runtime"
	"seanime/internal/database/models"
	"seanime/internal/torrents/torrent"
	"seanime/internal/util"
	"time"
)

// HandleGetSettings
//
//	@summary returns the app settings.
//	@route /api/v1/settings [GET]
//	@returns models.Settings
func HandleGetSettings(c *RouteCtx) error {

	settings, err := c.App.Database.GetSettings()
	if err != nil {
		return c.RespondWithError(err)
	}
	if settings.ID == 0 {
		return c.RespondWithError(errors.New(runtime.GOOS))
	}

	return c.RespondWithData(settings)
}

// HandleGettingStarted
//
//	@summary updates the app settings.
//	@desc This will update the app settings.
//	@desc The client should re-fetch the server status after this.
//	@route /api/v1/start [POST]
//	@returns handlers.Status
func HandleGettingStarted(c *RouteCtx) error {

	type body struct {
		Library                models.LibrarySettings      `json:"library"`
		MediaPlayer            models.MediaPlayerSettings  `json:"mediaPlayer"`
		Torrent                models.TorrentSettings      `json:"torrent"`
		Anilist                models.AnilistSettings      `json:"anilist"`
		Discord                models.DiscordSettings      `json:"discord"`
		Manga                  models.MangaSettings        `json:"manga"`
		Notifications          models.NotificationSettings `json:"notifications"`
		EnableTranscode        bool                        `json:"enableTranscode"`
		EnableTorrentStreaming bool                        `json:"enableTorrentStreaming"`
	}
	var b body

	if err := c.Fiber.BodyParser(&b); err != nil {
		return c.RespondWithError(err)
	}

	settings, err := c.App.Database.UpsertSettings(&models.Settings{
		BaseModel: models.BaseModel{
			ID:        1,
			UpdatedAt: time.Now(),
		},
		Library:       &b.Library,
		MediaPlayer:   &b.MediaPlayer,
		Torrent:       &b.Torrent,
		Anilist:       &b.Anilist,
		Discord:       &b.Discord,
		Manga:         &b.Manga,
		Notifications: &b.Notifications,
		AutoDownloader: &models.AutoDownloaderSettings{
			Provider:              b.Library.TorrentProvider,
			Interval:              5,
			Enabled:               false,
			DownloadAutomatically: true,
			EnableEnhancedQueries: true,
		},
	})

	if err != nil {
		return c.RespondWithError(err)
	}

	if b.EnableTorrentStreaming {
		go func() {
			defer util.HandlePanicThen(func() {})
			prev, found := c.App.Database.GetTorrentstreamSettings()
			if found {
				prev.Enabled = true
				_, _ = c.App.Database.UpsertTorrentstreamSettings(prev)
			}
		}()
	}

	if b.EnableTranscode {
		go func() {
			defer util.HandlePanicThen(func() {})
			prev, found := c.App.Database.GetMediastreamSettings()
			if found {
				prev.TranscodeEnabled = true
				_, _ = c.App.Database.UpsertMediastreamSettings(prev)
			}
		}()
	}

	c.App.WSEventManager.SendEvent("settings", settings)

	status := NewStatus(c)

	// Refresh modules that depend on the settings
	c.App.InitOrRefreshModules()

	return c.RespondWithData(status)
}

// HandleSaveSettings
//
//	@summary updates the app settings.
//	@desc This will update the app settings.
//	@desc The client should re-fetch the server status after this.
//	@route /api/v1/settings [PATCH]
//	@returns handlers.Status
func HandleSaveSettings(c *RouteCtx) error {

	type body struct {
		Library       models.LibrarySettings      `json:"library"`
		MediaPlayer   models.MediaPlayerSettings  `json:"mediaPlayer"`
		Torrent       models.TorrentSettings      `json:"torrent"`
		Anilist       models.AnilistSettings      `json:"anilist"`
		Discord       models.DiscordSettings      `json:"discord"`
		Manga         models.MangaSettings        `json:"manga"`
		Notifications models.NotificationSettings `json:"notifications"`
	}
	var b body

	if err := c.Fiber.BodyParser(&b); err != nil {
		return c.RespondWithError(err)
	}

	autoDownloaderSettings := models.AutoDownloaderSettings{}
	prevSettings, err := c.App.Database.GetSettings()
	if err == nil && prevSettings.AutoDownloader != nil {
		autoDownloaderSettings = *prevSettings.AutoDownloader
	}
	// Disable auto-downloader if the torrent provider is set to none
	if b.Library.TorrentProvider == torrent.ProviderNone && autoDownloaderSettings.Enabled {
		c.App.Logger.Debug().Msg("app: Disabling auto-downloader because the torrent provider is set to none")
		autoDownloaderSettings.Enabled = false
	}

	settings, err := c.App.Database.UpsertSettings(&models.Settings{
		BaseModel: models.BaseModel{
			ID:        1,
			UpdatedAt: time.Now(),
		},
		Library:        &b.Library,
		MediaPlayer:    &b.MediaPlayer,
		Torrent:        &b.Torrent,
		Anilist:        &b.Anilist,
		Manga:          &b.Manga,
		Discord:        &b.Discord,
		Notifications:  &b.Notifications,
		AutoDownloader: &autoDownloaderSettings,
	})

	if err != nil {
		return c.RespondWithError(err)
	}

	c.App.WSEventManager.SendEvent("settings", settings)

	status := NewStatus(c)

	// Refresh modules that depend on the settings
	c.App.InitOrRefreshModules()

	return c.RespondWithData(status)
}

// HandleSaveAutoDownloaderSettings
//
//	@summary updates the auto-downloader settings.
//	@route /api/v1/settings/auto-downloader [PATCH]
//	@returns bool
func HandleSaveAutoDownloaderSettings(c *RouteCtx) error {

	type body struct {
		Interval              int  `json:"interval"`
		Enabled               bool `json:"enabled"`
		DownloadAutomatically bool `json:"downloadAutomatically"`
		EnableEnhancedQueries bool `json:"enableEnhancedQueries"`
	}

	var b body

	if err := c.Fiber.BodyParser(&b); err != nil {
		return c.RespondWithError(err)
	}

	currSettings, err := c.App.Database.GetSettings()
	if err != nil {
		return c.RespondWithError(err)
	}

	// Validation
	if b.Interval < 2 {
		return c.RespondWithError(errors.New("interval must be at least 2 minutes"))
	}

	autoDownloaderSettings := &models.AutoDownloaderSettings{
		Provider:              currSettings.Library.TorrentProvider,
		Interval:              b.Interval,
		Enabled:               b.Enabled,
		DownloadAutomatically: b.DownloadAutomatically,
		EnableEnhancedQueries: b.EnableEnhancedQueries,
	}

	currSettings.AutoDownloader = autoDownloaderSettings
	currSettings.BaseModel = models.BaseModel{
		ID:        1,
		UpdatedAt: time.Now(),
	}

	_, err = c.App.Database.UpsertSettings(currSettings)
	if err != nil {
		return c.RespondWithError(err)
	}

	// Update Auto Downloader - This runs in a goroutine
	c.App.AutoDownloader.SetSettings(autoDownloaderSettings, currSettings.Library.TorrentProvider)

	return c.RespondWithData(true)
}
