package entities

import (
	"errors"
	"github.com/samber/lo"
	"github.com/seanime-app/seanime-server/internal/anilist"
	"github.com/seanime-app/seanime-server/internal/anizip"
	"github.com/sourcegraph/conc/pool"
	"strconv"
)

type (
	// MediaEntryInfo is instantiated by the MediaEntry
	MediaEntryInfo struct {
		EpisodesToDownload    []*MediaEntryDownloadInfo `json:"episodesToDownload"`
		CanBatch              bool                      `json:"canBatch"`
		BatchAll              bool                      `json:"batchAll"`
		HasInaccurateSchedule bool                      `json:"hasInaccurateSchedule"`
		Rewatch               bool                      `json:"rewatch"`
	}

	MediaEntryDownloadInfo struct {
		EpisodeNumber int                `json:"episodeNumber"`
		AniDBEpisode  string             `json:"aniDBEpisode"`
		Episode       *MediaEntryEpisode `json:"episode"`
	}

	NewMediaEntryInfoOptions struct {
		// Media's local files
		localFiles   []*LocalFile
		anizipMedia  *anizip.Media
		media        *anilist.BaseMedia
		anilistEntry *anilist.AnimeCollection_MediaListCollection_Lists_Entries
	}
)

// NewMediaEntryInfo creates a new MediaEntryInfo
func NewMediaEntryInfo(opts *NewMediaEntryInfoOptions) (*MediaEntryInfo, error) {

	if *opts.media.Status == anilist.MediaStatusNotYetReleased {
		return &MediaEntryInfo{}, nil
	}
	if opts.anizipMedia == nil {
		return nil, errors.New("could not get anizip media")
	}
	if opts.media.GetCurrentEpisodeCount() == -1 {
		return nil, errors.New("could not get current media episode count")
	}
	possibleSpecialInclusion, hasDiscrepancy := detectDiscrepancy(opts.localFiles, opts.media, opts.anizipMedia)

	// Get progress, if the media isn't in the user's list, progress is 0
	progress := 0
	if opts.anilistEntry != nil {
		// Set progress if entry exist
		progress = *opts.anilistEntry.GetProgress()
		// If the media is completed, set progress is 0
		if *opts.anilistEntry.Status == anilist.MediaListStatusCompleted {
			progress = 0
		}
	}

	// We will assume that Episode 0 is 1 if it is included by AniList
	mediaEpSlice := generateEpSlice(opts.media.GetCurrentEpisodeCount())
	unwatchedEpSlice := lo.Filter(mediaEpSlice, func(i int, _ int) bool { return i > progress })

	anizipEpSlice := generateEpSlice(opts.anizipMedia.GetMainEpisodeCount())
	unwatchedAnizipEpSlice := lo.Filter(anizipEpSlice, func(i int, _ int) bool { return i > progress })

	if hasDiscrepancy {
		// Add -1 to slice, -1 is "S1"
		anizipEpSlice = append([]int{-1}, anizipEpSlice...) // e.g, [-1,1,2,...,12]
		unwatchedAnizipEpSlice = anizipEpSlice
		if progress > 0 {
			// e.g, progress = 1 (0), -> ["S1",1,2,3,...,12]
			// e.g, progress = 2 (1), -> [1,2,3, ...,12]
			unwatchedAnizipEpSlice = lo.Filter(anizipEpSlice, func(i int, _ int) bool { return i > progress-1 })
		}
	}

	// Filter out unavailable episodes
	if opts.media.NextAiringEpisode != nil {
		unwatchedEpSlice = lo.Filter(unwatchedEpSlice, func(i int, _ int) bool { return i < opts.media.NextAiringEpisode.Episode })
		if hasDiscrepancy {
			unwatchedAnizipEpSlice = lo.Filter(unwatchedAnizipEpSlice, func(i int, _ int) bool { return i < opts.media.NextAiringEpisode.Episode-1 })
		} else {
			unwatchedAnizipEpSlice = lo.Filter(unwatchedAnizipEpSlice, func(i int, _ int) bool { return i < opts.media.NextAiringEpisode.Episode })
		}
	}

	// Inaccurate schedule
	hasInaccurateSchedule := false
	if opts.media.NextAiringEpisode == nil && *opts.media.Status == anilist.MediaStatusReleasing {
		if !hasDiscrepancy {
			if progress+1 < opts.anizipMedia.GetMainEpisodeCount() {
				unwatchedEpSlice = lo.Filter(unwatchedEpSlice, func(i int, _ int) bool { return i > progress && i <= progress+1 })
				unwatchedAnizipEpSlice = lo.Filter(unwatchedAnizipEpSlice, func(i int, _ int) bool { return i > progress && i <= progress+1 })
			} else {
				unwatchedEpSlice = lo.Filter(unwatchedEpSlice, func(i int, _ int) bool { return i > progress && i <= progress })
				unwatchedAnizipEpSlice = lo.Filter(unwatchedAnizipEpSlice, func(i int, _ int) bool { return i > progress && i <= progress })
			}
		} else {
			if progress+1 < opts.anizipMedia.GetMainEpisodeCount() {
				unwatchedEpSlice = lo.Filter(unwatchedEpSlice, func(i int, _ int) bool { return i > progress && i <= progress })
				unwatchedAnizipEpSlice = lo.Filter(unwatchedAnizipEpSlice, func(i int, _ int) bool { return i > progress && i <= progress })
			} else {
				unwatchedEpSlice = lo.Filter(unwatchedEpSlice, func(i int, _ int) bool { return i > progress && i <= progress-1 })
				unwatchedAnizipEpSlice = lo.Filter(unwatchedAnizipEpSlice, func(i int, _ int) bool { return i > progress && i <= progress-1 })
			}
		}
		hasInaccurateSchedule = true
	}

	// This slice contains episode numbers that are not downloaded
	// The source of truth is AniZip, but we will handle discrepancies
	toDownloadSlice := make([]int, 0)
	lfsEpSlice := make([]int, 0)
	if opts.localFiles != nil {

		// Get all episode numbers of main local files
		for _, lf := range opts.localFiles {
			if lf.Metadata.Type == LocalFileTypeMain {
				lfsEpSlice = append(lfsEpSlice, lf.Metadata.Episode)
			}
		}
		// If there is a discrepancy and local files include episode 0, add -1 ("S1") to slice
		if hasDiscrepancy && possibleSpecialInclusion {
			lfsEpSlice = lo.Filter(lfsEpSlice, func(i int, _ int) bool { return i != 0 })
			lfsEpSlice = append([]int{-1}, lfsEpSlice...) // e.g, [-1,1,2,...,12]
		}
		// Filter out downloaed episodes
		if len(lfsEpSlice) > 0 {
			toDownloadSlice = lo.Filter(unwatchedAnizipEpSlice, func(i int, _ int) bool {
				return !lo.Contains(lfsEpSlice, i)
			})
		} else {
			toDownloadSlice = unwatchedAnizipEpSlice
		}
	} else {
		toDownloadSlice = unwatchedAnizipEpSlice
	}

	//---------------------------------

	// Generate `episodesToDownload` based on `toDownloadSlice`
	//episodesToDownload := make([]*MediaEntryDownloadInfo, 0)
	p := pool.NewWithResults[*MediaEntryDownloadInfo]()
	for _, ep := range toDownloadSlice {
		ep := ep
		p.Go(func() *MediaEntryDownloadInfo {
			str := new(MediaEntryDownloadInfo)
			str.EpisodeNumber = ep
			str.AniDBEpisode = strconv.Itoa(ep)
			if ep == -1 {
				str.EpisodeNumber = 0
				str.AniDBEpisode = "S1"
			}
			str.Episode = NewMediaEntryEpisode(&NewMediaEntryEpisodeOptions{
				localFile:            nil,
				optionalAniDBEpisode: str.AniDBEpisode,
				anizipMedia:          opts.anizipMedia,
				media:                opts.media,
				progressOffset:       0,
				isDownloaded:         false,
			})
			return str
		})
	}
	episodesToDownload := p.Wait()

	//--------------

	canBatch := false
	if *opts.media.GetStatus() == anilist.MediaStatusFinished && opts.media.GetTotalEpisodeCount() > 0 {
		canBatch = true
	}
	batchAll := false
	if canBatch && len(lfsEpSlice) == 0 && progress == 0 {
		batchAll = true
	}
	rewatch := false
	if opts.anilistEntry != nil && *opts.anilistEntry.Status == anilist.MediaListStatusCompleted {
		rewatch = true
	}

	//println(spew.Sdump(episodesToDownload))
	//println(spew.Sprint(mediaEpSlice))
	//println(spew.Sprint(unwatchedEpSlice))
	//println(spew.Sprint(anizipEpSlice))
	//println(spew.Sprint(unwatchedAnizipEpSlice))

	return &MediaEntryInfo{
		EpisodesToDownload:    episodesToDownload,
		CanBatch:              canBatch,
		BatchAll:              batchAll,
		Rewatch:               rewatch,
		HasInaccurateSchedule: hasInaccurateSchedule,
	}, nil
}

// generateEpSlice
// e.g, 4 -> [1,2,3,4], 3 -> [1,2,3]
func generateEpSlice(n int) []int {
	if n < 1 {
		return nil
	}
	result := make([]int, n)
	for i := 1; i <= n; i++ {
		result[i-1] = i
	}
	return result
}
