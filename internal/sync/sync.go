package sync

import (
	"github.com/samber/lo"
	"github.com/samber/mo"
	"seanime/internal/api/anilist"
	"seanime/internal/api/metadata"
	"seanime/internal/library/anime"
	"seanime/internal/manga"
	"seanime/internal/util"
	"seanime/internal/util/result"
	"sync"
)

// DEVNOTE: The synchronization process is split into 3 parts:
// 1. ManagerImpl.synchronize removes outdated tracked anime & manga, runs Syncer.runDiffs and adds changed tracked anime & manga to the queue.
// 2. The Syncer processes the queue, calling Syncer.synchronizeAnime and Syncer.synchronizeManga for each job.
// 3. Syncer.synchronizeCollections creates a local collection that mirrors the remote collection, containing only the tracked anime & manga. Only called when the queue is emptied.

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type (
	// Syncer will synchronize the anime and manga snapshots in the local database.
	// Anytime Manager.Synchronize is called, tracked anime and manga will be added to the queue.
	// The queue will synchronize one anime and one manga every X minutes, until it's empty.
	//
	// Synchronization can fail due to network issues. When it does, the anime or manga will be added to the failed queue.
	Syncer struct {
		animeJobQueue chan AnimeJob
		mangaJobQueue chan MangaJob

		changedAnimeQueue *result.Cache[int, *AnimeDiffResult]
		changedMangaQueue *result.Cache[int, *MangaDiffResult]

		failedAnimeQueue *result.Cache[int, *anilist.AnimeListEntry]
		failedMangaQueue *result.Cache[int, *anilist.MangaListEntry]

		trackedAnimeMap map[int]*TrackedMedia
		trackedMangaMap map[int]*TrackedMedia

		manager *ManagerImpl
		mu      sync.Mutex

		shouldUpdateLocalCollections bool
		doneUpdatingLocalCollections chan struct{}
	}

	QueueProgress struct {
	}

	AnimeJob struct {
		Diff *AnimeDiffResult
	}
	MangaJob struct {
		Diff *MangaDiffResult
	}
)

func NewQueue(manager *ManagerImpl) *Syncer {
	ret := &Syncer{
		animeJobQueue:                make(chan AnimeJob, 100),
		mangaJobQueue:                make(chan MangaJob, 100),
		changedAnimeQueue:            result.NewCache[int, *AnimeDiffResult](),
		changedMangaQueue:            result.NewCache[int, *MangaDiffResult](),
		failedAnimeQueue:             result.NewCache[int, *anilist.AnimeListEntry](),
		failedMangaQueue:             result.NewCache[int, *anilist.MangaListEntry](),
		shouldUpdateLocalCollections: false,
		doneUpdatingLocalCollections: make(chan struct{}, 1),
		manager:                      manager,
	}

	go ret.processAnimeJobs()
	go ret.processMangaJobs()

	return ret
}

func (q *Syncer) processAnimeJobs() {
	for job := range q.animeJobQueue {
		q.shouldUpdateLocalCollections = true
		q.synchronizeAnime(job.Diff)
		q.checkAndUpdateLocalCollections()
	}
}

func (q *Syncer) processMangaJobs() {
	for job := range q.mangaJobQueue {
		q.shouldUpdateLocalCollections = true
		q.synchronizeManga(job.Diff)
		q.checkAndUpdateLocalCollections()
	}
}

func (q *Syncer) checkAndUpdateLocalCollections() {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if we need to update the local collections
	if q.shouldUpdateLocalCollections {
		// Check if both queues are empty
		if len(q.animeJobQueue) == 0 && len(q.mangaJobQueue) == 0 {
			// Update the local collections
			err := q.synchronizeCollections()
			if err != nil {
				q.manager.logger.Error().Err(err).Msg("sync: Failed to synchronize collections")
			}
			q.shouldUpdateLocalCollections = false
			q.doneUpdatingLocalCollections <- struct{}{}
		}
	}
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// synchronizeCollections should be called after the tracked anime & manga snapshots have been updated.
// The ManagerImpl.animeCollection and ManagerImpl.mangaCollection should be set & up-to-date.
// Instead of modifying the local collections directly, we create new collections that mirror the remote collections, but with up-to-date data.
func (q *Syncer) synchronizeCollections() (err error) {
	defer util.HandlePanicInModuleWithError("sync/synchronizeCollections", &err)

	// DEVNOTE: "_" prefix = original/remote collection
	// We shouldn't modify the remote collection, so making sure we get new pointers

	q.manager.logger.Trace().Msg("sync: Synchronizing local collections")

	_animeCollection := q.manager.animeCollection.MustGet()
	_mangaCollection := q.manager.mangaCollection.MustGet()

	// Get up-to-date snapshots
	animeSnapshots, _ := q.manager.localDb.GetAnimeSnapshots()
	mangaSnapshots, _ := q.manager.localDb.GetMangaSnapshots()

	animeSnapshotMap := make(map[int]*AnimeSnapshot)
	for _, snapshot := range animeSnapshots {
		animeSnapshotMap[snapshot.MediaId] = snapshot
	}

	mangaSnapshotMap := make(map[int]*MangaSnapshot)
	for _, snapshot := range mangaSnapshots {
		mangaSnapshotMap[snapshot.MediaId] = snapshot
	}

	localAnimeCollection := &anilist.AnimeCollection{
		MediaListCollection: &anilist.AnimeCollection_MediaListCollection{
			Lists: []*anilist.AnimeCollection_MediaListCollection_Lists{},
		},
	}

	localMangaCollection := &anilist.MangaCollection{
		MediaListCollection: &anilist.MangaCollection_MediaListCollection{
			Lists: []*anilist.MangaCollection_MediaListCollection_Lists{},
		},
	}

	// Re-create all anime collection lists, without entries
	for _, _animeList := range _animeCollection.MediaListCollection.GetLists() {
		if _animeList.GetStatus() == nil {
			continue
		}
		list := &anilist.AnimeCollection_MediaListCollection_Lists{
			Status:       ToNewPointer(_animeList.Status),
			Name:         ToNewPointer(_animeList.Name),
			IsCustomList: ToNewPointer(_animeList.IsCustomList),
			Entries:      []*anilist.AnimeListEntry{},
		}
		localAnimeCollection.MediaListCollection.Lists = append(localAnimeCollection.MediaListCollection.Lists, list)
	}

	// Re-create all manga collection lists, without entries
	for _, _mangaList := range _mangaCollection.MediaListCollection.GetLists() {
		if _mangaList.GetStatus() == nil {
			continue
		}
		list := &anilist.MangaCollection_MediaListCollection_Lists{
			Status:       ToNewPointer(_mangaList.Status),
			Name:         ToNewPointer(_mangaList.Name),
			IsCustomList: ToNewPointer(_mangaList.IsCustomList),
			Entries:      []*anilist.MangaListEntry{},
		}
		localMangaCollection.MediaListCollection.Lists = append(localMangaCollection.MediaListCollection.Lists, list)
	}

	//visited := make(map[int]struct{})

	if len(animeSnapshots) > 0 {
		// Create local anime collection
		for _, _animeList := range _animeCollection.MediaListCollection.GetLists() {
			if _animeList.GetStatus() == nil {
				continue
			}
			for _, _animeEntry := range _animeList.GetEntries() {
				// Check if the anime is tracked
				_, found := q.trackedAnimeMap[_animeEntry.GetMedia().GetID()]
				if !found {
					continue
				}
				// Get the anime snapshot
				snapshot, found := animeSnapshotMap[_animeEntry.GetMedia().GetID()]
				if !found {
					continue
				}

				// Add the anime to the right list
				for _, list := range localAnimeCollection.MediaListCollection.GetLists() {
					if list.GetStatus() == nil {
						continue
					}

					if *list.GetStatus() != *_animeList.GetStatus() {
						continue
					}

					editedAnime := BaseAnimeDeepCopy(_animeEntry.Media)
					editedAnime.BannerImage = FormatAssetUrl(snapshot.MediaId, snapshot.BannerImagePath)
					editedAnime.CoverImage = &anilist.BaseAnime_CoverImage{
						ExtraLarge: FormatAssetUrl(snapshot.MediaId, snapshot.CoverImagePath),
						Large:      FormatAssetUrl(snapshot.MediaId, snapshot.CoverImagePath),
						Medium:     FormatAssetUrl(snapshot.MediaId, snapshot.CoverImagePath),
						Color:      FormatAssetUrl(snapshot.MediaId, snapshot.CoverImagePath),
					}

					var startedAt *anilist.AnimeCollection_MediaListCollection_Lists_Entries_StartedAt
					if _animeEntry.StartedAt != nil {
						startedAt = &anilist.AnimeCollection_MediaListCollection_Lists_Entries_StartedAt{
							Year:  ToNewPointer(_animeEntry.StartedAt.GetYear()),
							Month: ToNewPointer(_animeEntry.StartedAt.GetMonth()),
							Day:   ToNewPointer(_animeEntry.StartedAt.GetDay()),
						}
					}

					var completedAt *anilist.AnimeCollection_MediaListCollection_Lists_Entries_CompletedAt
					if _animeEntry.CompletedAt != nil {
						completedAt = &anilist.AnimeCollection_MediaListCollection_Lists_Entries_CompletedAt{
							Year:  ToNewPointer(_animeEntry.CompletedAt.GetYear()),
							Month: ToNewPointer(_animeEntry.CompletedAt.GetMonth()),
							Day:   ToNewPointer(_animeEntry.CompletedAt.GetDay()),
						}
					}

					entry := &anilist.AnimeListEntry{
						ID:          _animeEntry.ID,
						Score:       ToNewPointer(_animeEntry.Score),
						Progress:    ToNewPointer(_animeEntry.Progress),
						Status:      ToNewPointer(_animeEntry.Status),
						Notes:       ToNewPointer(_animeEntry.Notes),
						Repeat:      ToNewPointer(_animeEntry.Repeat),
						Private:     ToNewPointer(_animeEntry.Private),
						StartedAt:   startedAt,
						CompletedAt: completedAt,
						Media:       editedAnime,
					}
					list.Entries = append(list.Entries, entry)
					break
				}

			}
		}
	}

	if len(mangaSnapshots) > 0 {
		// Create local manga collection
		for _, _mangaList := range _mangaCollection.MediaListCollection.GetLists() {
			if _mangaList.GetStatus() == nil {
				continue
			}
			for _, _mangaEntry := range _mangaList.GetEntries() {
				// Check if the manga is tracked
				_, found := q.trackedMangaMap[_mangaEntry.GetMedia().GetID()]
				if !found {
					continue
				}
				// Get the manga snapshot
				snapshot, found := mangaSnapshotMap[_mangaEntry.GetMedia().GetID()]
				if !found {
					continue
				}

				// Add the manga to the right list
				for _, list := range localMangaCollection.MediaListCollection.GetLists() {
					if list.GetStatus() == nil {
						continue
					}

					if *list.GetStatus() != *_mangaList.GetStatus() {
						continue
					}

					editedManga := BaseMangaDeepCopy(_mangaEntry.Media)
					editedManga.BannerImage = FormatAssetUrl(snapshot.MediaId, snapshot.BannerImagePath)
					editedManga.CoverImage = &anilist.BaseManga_CoverImage{
						ExtraLarge: FormatAssetUrl(snapshot.MediaId, snapshot.CoverImagePath),
						Large:      FormatAssetUrl(snapshot.MediaId, snapshot.CoverImagePath),
						Medium:     FormatAssetUrl(snapshot.MediaId, snapshot.CoverImagePath),
						Color:      FormatAssetUrl(snapshot.MediaId, snapshot.CoverImagePath),
					}

					var startedAt *anilist.MangaCollection_MediaListCollection_Lists_Entries_StartedAt
					if _mangaEntry.StartedAt != nil {
						startedAt = &anilist.MangaCollection_MediaListCollection_Lists_Entries_StartedAt{
							Year:  ToNewPointer(_mangaEntry.StartedAt.GetYear()),
							Month: ToNewPointer(_mangaEntry.StartedAt.GetMonth()),
							Day:   ToNewPointer(_mangaEntry.StartedAt.GetDay()),
						}
					}

					var completedAt *anilist.MangaCollection_MediaListCollection_Lists_Entries_CompletedAt
					if _mangaEntry.CompletedAt != nil {
						completedAt = &anilist.MangaCollection_MediaListCollection_Lists_Entries_CompletedAt{
							Year:  ToNewPointer(_mangaEntry.CompletedAt.GetYear()),
							Month: ToNewPointer(_mangaEntry.CompletedAt.GetMonth()),
							Day:   ToNewPointer(_mangaEntry.CompletedAt.GetDay()),
						}
					}

					entry := &anilist.MangaListEntry{
						ID:          _mangaEntry.ID,
						Score:       ToNewPointer(_mangaEntry.Score),
						Progress:    ToNewPointer(_mangaEntry.Progress),
						Status:      ToNewPointer(_mangaEntry.Status),
						Notes:       ToNewPointer(_mangaEntry.Notes),
						Repeat:      ToNewPointer(_mangaEntry.Repeat),
						Private:     ToNewPointer(_mangaEntry.Private),
						StartedAt:   startedAt,
						CompletedAt: completedAt,
						Media:       editedManga,
					}
					list.Entries = append(list.Entries, entry)
					break
				}

			}
		}
	}

	// Save the local collections
	err = q.manager.localDb.SaveAnimeCollection(localAnimeCollection)
	if err != nil {
		return err
	}
	q.manager.localAnimeCollection = mo.Some(localAnimeCollection)

	err = q.manager.localDb.SaveMangaCollection(localMangaCollection)
	if err != nil {
		return err
	}
	q.manager.localMangaCollection = mo.Some(localMangaCollection)

	q.manager.logger.Debug().Msg("sync: Synchronized local collections")

	return nil
}

//----------------------------------------------------------------------------------------------------------------------------------------------------

func (q *Syncer) sendAnimeToFailedQueue(entry *anilist.AnimeListEntry) {
	// TODO: Maybe send an event to the client
	q.failedAnimeQueue.Set(entry.Media.ID, entry)
}

func (q *Syncer) sendMangaToFailedQueue(entry *anilist.MangaListEntry) {

	q.failedMangaQueue.Set(entry.Media.ID, entry)
}

//----------------------------------------------------------------------------------------------------------------------------------------------------

// runDiffs runs the diffing process to find outdated anime & manga.
// The diffs are then added to the changedAnimeQueue and changedMangaQueue.
func (q *Syncer) runDiffs(
	trackedAnimeMap map[int]*TrackedMedia,
	trackedAnimeSnapshotMap map[int]*AnimeSnapshot,
	trackedMangaMap map[int]*TrackedMedia,
	trackedMangaSnapshotMap map[int]*MangaSnapshot,
	localFiles []*anime.LocalFile,
	downloadedChapterContainers []*manga.ChapterContainer,
) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.manager.logger.Trace().Msg("sync: Running diffs")

	if q.manager.animeCollection.IsAbsent() {
		q.manager.logger.Error().Msg("sync: Cannot get diffs, anime collection is absent")
		return
	}

	if q.manager.mangaCollection.IsAbsent() {
		q.manager.logger.Error().Msg("sync: Cannot get diffs, manga collection is absent")
		return
	}

	if len(q.animeJobQueue) > 0 || len(q.mangaJobQueue) > 0 {
		q.manager.logger.Trace().Msg("sync: Skipping diffs, job queues are not empty")
		return
	}

	diff := &Diff{
		Logger: q.manager.logger,
	}

	q.trackedAnimeMap = trackedAnimeMap
	q.trackedMangaMap = trackedMangaMap

	wg := sync.WaitGroup{}
	wg.Add(2)

	var animeDiffs map[int]*AnimeDiffResult

	go func() {
		animeDiffs = diff.GetAnimeDiffs(GetAnimeDiffOptions{
			Collection:      q.manager.animeCollection.MustGet(),
			LocalCollection: q.manager.localAnimeCollection,
			LocalFiles:      localFiles,
			TrackedAnime:    trackedAnimeMap,
			Snapshots:       trackedAnimeSnapshotMap,
		})
		wg.Done()
		q.manager.logger.Trace().Msg("sync: Finished getting anime diffs")
	}()

	var mangaDiffs map[int]*MangaDiffResult

	go func() {
		mangaDiffs = diff.GetMangaDiffs(GetMangaDiffOptions{
			Collection:                  q.manager.mangaCollection.MustGet(),
			LocalCollection:             q.manager.localMangaCollection,
			DownloadedChapterContainers: downloadedChapterContainers,
			TrackedManga:                trackedMangaMap,
			Snapshots:                   trackedMangaSnapshotMap,
		})
		wg.Done()
		q.manager.logger.Trace().Msg("sync: Finished getting manga diffs")
	}()

	wg.Wait()

	// Add the diffs to be synced asynchronously
	go func() {
		q.manager.logger.Trace().Int("animeJobs", len(animeDiffs)).Int("mangaJobs", len(mangaDiffs)).Msg("sync: Adding diffs to the job queues")

		for _, i := range animeDiffs {
			q.animeJobQueue <- AnimeJob{Diff: i}
		}
		for _, i := range mangaDiffs {
			q.mangaJobQueue <- MangaJob{Diff: i}
		}
	}()

	// Done
	q.manager.logger.Trace().Msg("sync: Done running diffs")
}

//----------------------------------------------------------------------------------------------------------------------------------------------------

// synchronizeAnime creates or updates the anime snapshot in the local database.
// The anime should be tracked.
//   - If the anime has no local files, it will be removed entirely from the local database.
//   - If the anime has local files, we create or update the snapshot.
func (q *Syncer) synchronizeAnime(diff *AnimeDiffResult) {
	defer util.HandlePanicInModuleThen("sync/synchronizeAnime", func() {})

	entry := diff.AnimeEntry

	if entry == nil {
		return
	}

	q.manager.logger.Trace().Msgf("sync: Starting synchronization of anime %d, diff type: %+v", entry.Media.ID, diff.DiffType)

	_, foundLocalFiles := lo.Find(q.manager.localFiles, func(f *anime.LocalFile) bool {
		return f.MediaId == entry.Media.ID
	})

	// If the anime (which is tracked) has no local files, remove it entirely from the local database
	if !foundLocalFiles {
		q.manager.logger.Warn().Msgf("sync: No local files found for anime %d, removing from the local database", entry.Media.ID)
		_ = q.manager.removeAnime(entry.Media.ID)
		return
	}

	var animeMetadata *metadata.AnimeMetadata
	if diff.DiffType == DiffTypeMissing || diff.DiffType == DiffTypeMetadata {
		// Get the anime metadata
		var err error
		animeMetadata, err = q.manager.metadataProvider.GetAnimeMetadata(metadata.AnilistPlatform, entry.Media.ID)
		if err != nil {
			q.sendAnimeToFailedQueue(entry)
			q.manager.logger.Error().Err(err).Msgf("sync: Failed to get metadata for anime %d", entry.Media.ID)
			return
		}
	}

	//
	// The snapshot is missing
	//
	if diff.DiffType == DiffTypeMissing {
		bannerImage, coverImage, episodeImagePaths, ok := DownloadAnimeImages(q.manager.logger, q.manager.localAssetsDir, entry, animeMetadata)
		if !ok {
			q.sendAnimeToFailedQueue(entry)
			return
		}

		// Create a new snapshot
		snapshot := &AnimeSnapshot{
			MediaId:           entry.Media.ID,
			AnimeMetadata:     LocalAnimeMetadata(*animeMetadata),
			BannerImagePath:   bannerImage,
			CoverImagePath:    coverImage,
			EpisodeImagePaths: episodeImagePaths,
			ReferenceKey:      GetAnimeReferenceKey(entry.Media, q.manager.localFiles),
		}

		// Save the snapshot
		err := q.manager.localDb.SaveAnimeSnapshot(snapshot)
		if err != nil {
			q.sendAnimeToFailedQueue(entry)
			q.manager.logger.Error().Err(err).Msgf("sync: Failed to save anime snapshot for anime %d", entry.Media.ID)
		}
		return
	}

	//
	// The snapshot metadata is outdated (local files have changed)
	// Update the anime metadata & download the new episode images if needed
	//
	if diff.DiffType == DiffTypeMetadata && diff.AnimeSnapshot != nil {

		snapshot := *diff.AnimeSnapshot
		snapshot.AnimeMetadata = LocalAnimeMetadata(*animeMetadata)
		snapshot.ReferenceKey = GetAnimeReferenceKey(entry.Media, q.manager.localFiles)

		// Get the current episode image URLs
		currentEpisodeImageUrls := make(map[string]string)
		for episodeNum, episode := range animeMetadata.Episodes {
			if episode.Image == "" {
				continue
			}
			currentEpisodeImageUrls[episodeNum] = episode.Image
		}

		// Get the episode image URLs that we need to download (i.e. the ones that are not in the snapshot)
		episodeImageUrlsToDownload := make(map[string]string)
		// For each current episode image URL, check if the key (episode number) is in the snapshot
		for episodeNum, episodeImageUrl := range currentEpisodeImageUrls {
			if _, found := snapshot.EpisodeImagePaths[episodeNum]; !found {
				episodeImageUrlsToDownload[episodeNum] = episodeImageUrl
			}
		}

		// Download the episode images if needed
		if len(episodeImageUrlsToDownload) > 0 {
			// Download only the episode images that we need to download
			episodeImagePaths, ok := DownloadAnimeEpisodeImages(q.manager.logger, q.manager.localAssetsDir, entry.Media.ID, episodeImageUrlsToDownload)
			if !ok {
				// DownloadAnimeEpisodeImages will log the error
				q.sendAnimeToFailedQueue(entry)
				return
			}
			// Update the snapshot by adding the new episode images
			for episodeNum, episodeImagePath := range episodeImagePaths {
				snapshot.EpisodeImagePaths[episodeNum] = episodeImagePath
			}
		}

		// Save the snapshot
		err := q.manager.localDb.SaveAnimeSnapshot(&snapshot)
		if err != nil {
			q.sendAnimeToFailedQueue(entry)
			q.manager.logger.Error().Err(err).Msgf("sync: Failed to save anime snapshot for anime %d", entry.Media.ID)
		}
		return
	}

	// The snapshot is up-to-date
	return
}

// synchronizeManga creates or updates the manga snapshot in the local database.
// We know that the manga is tracked.
//   - If the manga has no chapter containers, it will be removed entirely from the local database.
//   - If the manga has chapter containers, we create or update the snapshot.
func (q *Syncer) synchronizeManga(diff *MangaDiffResult) {
	defer util.HandlePanicInModuleThen("sync/synchronizeManga", func() {})

	entry := diff.MangaEntry

	if entry == nil {
		return
	}

	q.manager.logger.Trace().Msgf("sync: Starting synchronization of manga %d, diff type: %+v", entry.Media.ID, diff.DiffType)

	if q.manager.mangaCollection.IsAbsent() {
		return
	}

	eContainers := make([]*manga.ChapterContainer, 0)

	// Get the manga
	listEntry, ok := q.manager.mangaCollection.MustGet().GetListEntryFromMangaId(entry.Media.ID)
	if !ok {
		q.manager.logger.Error().Msgf("sync: Failed to get manga")
		return
	}

	if listEntry.GetStatus() == nil {
		return
	}

	// Get all chapter containers for this manga
	// A manga entry can have multiple chapter containers due to different sources
	for _, c := range q.manager.downloadedChapterContainers {
		if c.MediaId == entry.Media.ID {
			eContainers = append(eContainers, c)
		}
	}

	// If there are no chapter containers (they may have been deleted), remove the manga from the local database
	if len(eContainers) == 0 {
		_ = q.manager.removeManga(entry.Media.ID)
		return
	}

	if diff.DiffType == DiffTypeMissing {
		bannerImage, coverImage, ok := DownloadMangaImages(q.manager.logger, q.manager.localAssetsDir, entry)
		if !ok {
			q.sendMangaToFailedQueue(entry)
			return
		}

		// Create a new snapshot
		snapshot := &MangaSnapshot{
			MediaId:           entry.Media.ID,
			ChapterContainers: eContainers,
			BannerImagePath:   bannerImage,
			CoverImagePath:    coverImage,
			ReferenceKey:      GetMangaReferenceKey(entry.Media, eContainers),
		}

		// Save the snapshot
		err := q.manager.localDb.SaveMangaSnapshot(snapshot)
		if err != nil {
			q.sendMangaToFailedQueue(entry)
			q.manager.logger.Error().Err(err).Msgf("sync: Failed to save manga snapshot for manga %d", entry.Media.ID)
		}
		return
	}

	if diff.DiffType == DiffTypeMetadata && diff.MangaSnapshot != nil {
		snapshot := *diff.MangaSnapshot

		// Update the snapshot
		snapshot.ChapterContainers = eContainers
		snapshot.ReferenceKey = GetMangaReferenceKey(entry.Media, eContainers)

		// Save the snapshot
		err := q.manager.localDb.SaveMangaSnapshot(&snapshot)
		if err != nil {
			q.sendMangaToFailedQueue(entry)
			q.manager.logger.Error().Err(err).Msgf("sync: Failed to save manga snapshot for manga %d", entry.Media.ID)
		}
		return
	}

	// The snapshot is up-to-date
	return
}
