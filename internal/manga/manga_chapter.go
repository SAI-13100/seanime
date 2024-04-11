package manga

import (
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/samber/lo"
	chapter_downloader "github.com/seanime-app/seanime/internal/manga/downloader"
	"github.com/seanime-app/seanime/internal/manga/providers"
	"github.com/seanime-app/seanime/internal/util"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrNoResults            = errors.New("no results found for this media")
	ErrNoChapters           = errors.New("no manga chapters found")
	ErrChapterNotFound      = errors.New("chapter not found")
	ErrChapterNotDownloaded = errors.New("chapter not downloaded")
)

type (
	// ChapterContainer is used to display the list of chapters from a provider in the client.
	// It is cached in the file cache.
	ChapterContainer struct {
		MediaId  int                               `json:"mediaId"`
		Provider string                            `json:"provider"`
		Chapters []*manga_providers.ChapterDetails `json:"chapters"`
	}

	// PageContainer is used to display the list of pages from a chapter in the client.
	// It is cached in the file cache.
	PageContainer struct {
		MediaId        int                            `json:"mediaId"`
		Provider       string                         `json:"provider"`
		ChapterId      string                         `json:"chapterId"`
		Pages          []*manga_providers.ChapterPage `json:"pages"`
		PageDimensions map[int]*PageDimension         `json:"pageDimensions"` // Indexed by page number
		IsDownloaded   bool                           `json:"isDownloaded"`   // TODO
	}

	// PageDimension is used to store the dimensions of a page.
	// It is used by the client for 'Double Page' mode.
	PageDimension struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
)

// GetMangaChapterContainer returns the ChapterContainer for a manga entry based on the provider.
// If it isn't cached, it will search for the manga, create a ChapterContainer and cache it.
func (r *Repository) GetMangaChapterContainer(provider manga_providers.Provider, mediaId int, titles []*string) (*ChapterContainer, error) {

	key := fmt.Sprintf("%s$%d", provider, mediaId)

	r.logger.Debug().
		Str("provider", string(provider)).
		Int("mediaId", mediaId).
		Str("key", key).
		Msgf("manga: getting chapters")

	var container *ChapterContainer

	bucket := r.getFcProviderBucket(provider, mediaId, bucketTypeChapter)

	// Check if the container is in the cache
	if found, _ := r.fileCacher.Get(bucket, key, &container); found {
		r.logger.Info().Str("key", key).Msg("manga: Chapter Container Cache HIT")
		return container, nil
	}

	titles = lo.Filter(titles, func(title *string, _ int) bool {
		return util.IsMostlyLatinString(*title)
	})

	// 1. Search

	var searchRes []*manga_providers.SearchResult

	var err error
	for _, title := range titles {
		var _searchRes []*manga_providers.SearchResult
		switch provider {
		case manga_providers.ComickProvider:
			_searchRes, err = r.comick.Search(manga_providers.SearchOptions{
				Query: *title,
			})
		case manga_providers.MangaseeProvider:
			_searchRes, err = r.mangasee.Search(manga_providers.SearchOptions{
				Query: *title,
			})
		}
		if err == nil {
			searchRes = append(searchRes, _searchRes...)
		} else {
			r.logger.Warn().Err(err).Msg("manga: search failed")
		}
	}

	if searchRes == nil || len(searchRes) == 0 {
		r.logger.Error().Msg("manga: no search results found")
		return nil, ErrNoResults
	}

	// 2. Get chapters
	bestRes := searchRes[0]
	for _, res := range searchRes {
		if res.SearchRating > bestRes.SearchRating {
			bestRes = res
		}
	}

	var chapterList []*manga_providers.ChapterDetails

	switch provider {
	case manga_providers.ComickProvider:
		chapterList, err = r.comick.FindChapters(bestRes.ID)
	case manga_providers.MangaseeProvider:
		chapterList, err = r.mangasee.FindChapters(bestRes.ID)
	}

	if err != nil {
		r.logger.Error().Err(err).Msg("manga: find chapters failed")
		return nil, ErrNoChapters
	}

	container = &ChapterContainer{
		MediaId:  mediaId,
		Provider: string(provider),
		Chapters: chapterList,
	}

	// DEVNOTE: This might cache container with empty chapters, however the user can reload sources so it's fine
	err = r.fileCacher.Set(bucket, key, container)
	if err != nil {
		r.logger.Warn().Err(err).Msg("manga: failed to set cache")
	}

	r.logger.Info().Str("key", key).Msg("manga: chapters retrieved")

	return container, nil
}

// +-------------------------------------------------------------------------------------------------------------------+

// GetMangaPageContainer returns the PageContainer for a manga chapter based on the provider.
func (r *Repository) GetMangaPageContainer(
	provider manga_providers.Provider,
	mediaId int,
	chapterId string,
	doublePage bool,
) (*PageContainer, error) {

	//
	// Check downloads
	//

	ret, _ := r.GetDownloadedMangaPageContainer(provider, mediaId, chapterId)
	if ret != nil {
		return ret, nil
	}

	//
	//
	//

	// PageContainer key
	key := fmt.Sprintf("%s$%d$%s", provider, mediaId, chapterId)

	r.logger.Debug().
		Str("provider", string(provider)).
		Int("mediaId", mediaId).
		Str("key", key).
		Str("chapterId", chapterId).
		Msgf("manga: getting pages")

	var container *PageContainer

	// PageContainer bucket
	// e.g., manga_comick_pages_123
	//         -> { "comick$123$10010": PageContainer }, { "comick$123$10011": PageContainer }
	bucket := r.getFcProviderBucket(provider, mediaId, bucketTypePage)

	// Check if the container is in the cache
	if found, _ := r.fileCacher.Get(bucket, key, &container); found {

		// Hydrate page dimensions
		pageDimensions, _ := r.getPageDimensions(doublePage, string(provider), mediaId, chapterId, container.Pages)
		container.PageDimensions = pageDimensions

		r.logger.Info().Str("key", key).Msg("manga: Page Container Cache HIT")
		return container, nil
	}

	// Search for the chapter in the cache
	chapterBucket := r.getFcProviderBucket(provider, mediaId, bucketTypeChapter)

	var chapterContainer *ChapterContainer
	if found, _ := r.fileCacher.Get(chapterBucket, fmt.Sprintf("%s$%d", provider, mediaId), &chapterContainer); !found {
		r.logger.Error().Msg("manga: chapter container not found")
		return nil, ErrNoChapters
	}

	// Get the chapter from the container
	var chapter *manga_providers.ChapterDetails
	for _, c := range chapterContainer.Chapters {
		if c.ID == chapterId {
			chapter = c
			break
		}
	}

	if chapter == nil {
		r.logger.Error().Msg("manga: chapter not found")
		return nil, ErrChapterNotFound
	}

	// Get the chapter pages
	var pageList []*manga_providers.ChapterPage
	var err error

	switch provider {
	case manga_providers.ComickProvider:
		pageList, err = r.comick.FindChapterPages(chapter.ID)
	case manga_providers.MangaseeProvider:
		pageList, err = r.mangasee.FindChapterPages(chapter.ID)
	}

	if err != nil {
		r.logger.Error().Err(err).Msg("manga: could not get chapter pages")
		return nil, err
	}

	pageDimensions, _ := r.getPageDimensions(doublePage, string(provider), mediaId, chapterId, pageList)

	container = &PageContainer{
		MediaId:        mediaId,
		Provider:       string(provider),
		ChapterId:      chapterId,
		Pages:          pageList,
		PageDimensions: pageDimensions,
		IsDownloaded:   false,
	}

	// Set cache
	err = r.fileCacher.Set(bucket, key, container)
	if err != nil {
		r.logger.Warn().Err(err).Msg("manga: failed to set cache")
	}

	r.logger.Info().Str("key", key).Msg("manga: pages retrieved")

	return container, nil
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (r *Repository) getPageDimensions(enabled bool, provider string, mediaId int, chapterId string, pages []*manga_providers.ChapterPage) (ret map[int]*PageDimension, err error) {
	util.HandlePanicInModuleThen("manga/getPageDimensions", func() {
		err = fmt.Errorf("failed to get page dimensions")
	})

	if !enabled {
		return nil, nil
	}

	key := fmt.Sprintf("%s$%d$%s", provider, mediaId, chapterId)

	// Page dimensions bucket
	// e.g., manga_comick_page-dimensions_123
	//         -> { "comick$123$10010": PageDimensions }, { "comick$123$10011": PageDimensions }
	bucket := r.getFcProviderBucket(manga_providers.Provider(provider), mediaId, bucketTypePageDimensions)

	if found, _ := r.fileCacher.Get(bucket, fmt.Sprintf(key, provider, mediaId), &ret); found {
		r.logger.Info().Str("key", key).Msg("manga: Page Dimensions Cache HIT")
		return
	}

	r.logger.Debug().Str("key", key).Msg("manga: getting page dimensions")

	// Get the page dimensions
	pageDimensions := make(map[int]*PageDimension)
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	for _, page := range pages {
		wg.Add(1)
		go func(page *manga_providers.ChapterPage) {
			defer wg.Done()
			width, height, err := getImageNaturalSize(page.URL)
			if err != nil {
				//r.logger.Warn().Err(err).Int("index", page.Index).Msg("manga: failed to get image size")
				return
			}

			mu.Lock()
			// DEVNOTE: Index by page number
			pageDimensions[page.Index] = &PageDimension{
				Width:  width,
				Height: height,
			}
			mu.Unlock()
		}(page)
	}
	wg.Wait()

	_ = r.fileCacher.Set(bucket, key, pageDimensions)

	r.logger.Info().Str("key", key).Msg("manga: page dimensions retrieved")

	return pageDimensions, nil
}

// ----------------------------------------------------

// GetDownloadedMangaPageContainer returns the PageContainer for a downloaded manga chapter based on the provider.
func (r *Repository) GetDownloadedMangaPageContainer(
	provider manga_providers.Provider,
	mediaId int,
	chapterId string,
) (*PageContainer, error) {

	// Check if the chapter is downloaded
	found := false

	// Read download directory
	files, err := os.ReadDir(r.downloadDir)
	if err != nil {
		r.logger.Error().Err(err).Msg("manga: Failed to read download directory")
		return nil, err
	}

	chapterDir := "" // e.g., manga_comick_123_10010_13
	for _, file := range files {
		if file.IsDir() {
			parts := strings.SplitN(file.Name(), "_", 4)
			if len(parts) != 4 {
				continue
			}

			mId, _ := strconv.Atoi(parts[1])

			if parts[0] == string(provider) && mId == mediaId && parts[2] == chapterId {
				found = true
				chapterDir = file.Name()
				break
			}
		}
	}

	if !found {
		return nil, ErrChapterNotDownloaded
	}

	r.logger.Debug().Msg("manga: Found downloaded chapter directory")

	// Open registry file
	registryFile, err := os.Open(filepath.Join(r.downloadDir, chapterDir, "registry.json"))
	if err != nil {
		r.logger.Error().Err(err).Msg("manga: Failed to open registry file")
		return nil, err
	}
	defer registryFile.Close()

	r.logger.Info().Str("chapterId", chapterId).Msg("manga: Reading registry file")

	// Read registry file
	var pageRegistry *chapter_downloader.Registry
	err = json.NewDecoder(registryFile).Decode(&pageRegistry)
	if err != nil {
		r.logger.Error().Err(err).Msg("manga: Failed to decode registry file")
		return nil, err
	}

	pageList := make([]*manga_providers.ChapterPage, 0)
	pageDimensions := make(map[int]*PageDimension)

	// Get the downloaded pages
	for pageIndex, pageInfo := range *pageRegistry {
		pageList = append(pageList, &manga_providers.ChapterPage{
			Index:    pageIndex,
			URL:      filepath.Join(chapterDir, pageInfo.Filename),
			Provider: provider,
		})
		pageDimensions[pageIndex] = &PageDimension{
			Width:  pageInfo.Width,
			Height: pageInfo.Height,
		}
	}

	container := &PageContainer{
		MediaId:        mediaId,
		Provider:       string(provider),
		ChapterId:      chapterId,
		Pages:          pageList,
		PageDimensions: pageDimensions,
		IsDownloaded:   true,
	}

	r.logger.Info().Str("chapterId", chapterId).Msg("manga: Found downloaded chapter")

	return container, nil
}
