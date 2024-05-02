package videofile

import (
	"fmt"
	"github.com/coding-socks/ebml"
	"github.com/coding-socks/matroska"
	"github.com/davecgh/go-spew/spew"
	"github.com/goccy/go-json"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	"github.com/seanime-app/seanime/internal/util/result"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type MediaInfo struct {
	// closed if the mediainfo is ready for read. open otherwise
	ready <-chan struct{}
	// The sha1 of the video file
	Sha string `json:"sha"`
	// The internal path of the video file
	Path string `json:"path"`
	// The extension currently used to store this video file
	Extension string `json:"extension"`
	// The file size of the video file
	Size uint64 `json:"size"`
	// The length of the media in seconds
	Duration float32 `json:"duration"`
	// The container of the video file of this episode
	Container *string `json:"container"`
	// The video codec and information
	Video *Video `json:"video"`
	// The list of videos if there are multiples
	Videos []Video `json:"videos"`
	// The list of audio tracks
	Audios []Audio `json:"audios"`
	// The list of subtitles tracks
	Subtitles []Subtitle `json:"subtitles"`
	// The list of fonts that can be used to display subtitles
	Fonts []string `json:"fonts"`
	// The list of chapters. See Chapter for more information
	Chapters []Chapter `json:"chapters"`
}

type Video struct {
	// The codec of this stream (defined as the RFC 6381)
	Codec string `json:"codec"`
	// The language of this stream (as a ISO-639-2 language code)
	Language *string `json:"language"`
	// The max quality of this video track
	Quality Quality `json:"quality"`
	// The width of the video stream
	Width uint32 `json:"width"`
	// The height of the video stream
	Height uint32 `json:"height"`
	// The average bitrate of the video in bytes/s
	Bitrate uint32 `json:"bitrate"`
}

type Audio struct {
	// The index of this track on the media
	Index uint32 `json:"index"`
	// The title of the stream
	Title *string `json:"title"`
	// The language of this stream (as a ISO-639-2 language code)
	Language *string `json:"language"`
	// The codec of this stream
	Codec string `json:"codec"`
	// Is this stream the default one of its type?
	IsDefault bool `json:"isDefault"`
	// Is this stream tagged as forced? (useful only for subtitles)
	IsForced bool   `json:"isForced"`
	Channels uint32 `json:"channels"`
}

type Subtitle struct {
	// The index of this track on the media
	Index uint32 `json:"index"`
	// The title of the stream
	Title *string `json:"title"`
	// The language of this stream (as a ISO-639-2 language code)
	Language *string `json:"language"`
	// The codec of this stream
	Codec string `json:"codec"`
	// The extension for the codec
	Extension *string `json:"extension"`
	// Is this stream the default one of its type?
	IsDefault bool `json:"isDefault"`
	// Is this stream tagged as forced? (useful only for subtitles)
	IsForced bool `json:"isForced"`
	// The link to access this subtitle
	Link *string `json:"link"`
}

type Chapter struct {
	// The start time of the chapter (in second from the start of the episode)
	StartTime float32 `json:"startTime"`
	// The end time of the chapter (in second from the start of the episode)
	EndTime float32 `json:"endTime"`
	// The name of this chapter. This should be a human-readable name that could be presented to the user
	Name string `json:"name"`
	// TODO: add a type field for Opening, Credits...
}

type MediaInfoExtractor struct {
	sha    string
	path   string
	route  string
	logger *zerolog.Logger
}

func NewMediaInfoExtractor(path string, logger *zerolog.Logger) (*MediaInfoExtractor, error) {

	hash, err := GetHashFromPath(path)
	if err != nil {
		return nil, err
	}

	me := &MediaInfoExtractor{
		sha:    hash,
		path:   path,
		logger: logger,
	}

	return me, nil
}

var infos = result.NewCache[string, *MediaInfo]()

func (e *MediaInfoExtractor) GetInfo(metadataCachePath string) (mi *MediaInfo, err error) {
	readyChan := make(chan struct{})
	mi = &MediaInfo{
		Sha:   e.sha,
		ready: readyChan,
	}

	go func() {
		savePath := fmt.Sprintf("%s/%s/info.json", metadataCachePath, e.sha)
		if err := getSavedInfo(savePath, mi); err == nil {
			e.logger.Trace().Str("path", e.path).Msgf("Using mediainfo cache on filesystem")
			close(readyChan)
			return
		}

		var data *MediaInfo
		data, err = e.getInfo()
		*mi = *data
		mi.ready = readyChan
		mi.Sha = e.sha
		close(readyChan)
		saveInfo(savePath, mi)
	}()
	<-mi.ready
	return
}

func getSavedInfo[T any](savePath string, mi *T) error {
	savedFile, err := os.Open(savePath)
	if err != nil {
		return err
	}
	saved, err := io.ReadAll(savedFile)
	if err != nil {
		return err
	}
	err = json.Unmarshal(saved, mi)
	if err != nil {
		return err
	}
	return nil
}

func saveInfo[T any](savePath string, mi *T) error {
	content, err := json.Marshal(*mi)
	if err != nil {
		return err
	}
	// create directory if it doesn't exist
	_ = os.MkdirAll(filepath.Dir(savePath), 0755)
	return os.WriteFile(filepath.ToSlash(savePath), content, 0666)
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (e *MediaInfoExtractor) getInfo() (*MediaInfo, error) {

	// Open file
	file, err := os.Open(e.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Get file info
	fInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Matroska scanner
	scanner, err := matroska.NewScanner(file)
	if err != nil {
		return nil, err
	}
	info := scanner.Info()
	tracks := scanner.Tracks()

	// Get extension (e.g., ".mkv")
	ext := filepath.Ext(e.path)[1:]
	// Get size (in bytes)
	size := uint64(fInfo.Size())
	// Get duration (in seconds)
	duration := 0.0
	if info.Duration != nil {
		duration = *info.Duration / 1000
	}
	// Estimate bitrate
	bitrate := 0.0
	if duration > 0 {
		bitrate = float64(size) / duration
	}

	mi := &MediaInfo{
		Sha:       e.sha,
		Path:      e.path,
		Extension: ext,
		Size:      size,
		Duration:  float32(duration),
		Container: lo.ToPtr("matroska"),
	}

	//
	// Get advanced info \/
	//

	videos := make([]Video, 0)
	audios := make([]Audio, 0)
	subtitles := make([]Subtitle, 0)
	// Go through track entries
	audioIndex := 0
	subtitleIndex := 0

	for _, entry := range tracks.TrackEntry {
		//
		// Video
		//

		if entry.TrackType == matroska.TrackTypeVideo {
			spew.Dump(entry)
			v := &Video{
				Codec:    entry.CodecID,
				Width:    uint32(entry.Video.PixelWidth),
				Height:   uint32(entry.Video.PixelHeight),
				Bitrate:  uint32(bitrate),
				Language: &entry.Language,
				Quality:  GetQualityFromHeight(uint32(entry.Video.PixelHeight)),
			}
			videos = append(videos, *v)
		}
		//
		// Audio
		//
		if entry.TrackType == matroska.TrackTypeAudio {
			a := &Audio{
				Title:     entry.Name,
				Index:     uint32(audioIndex),
				Codec:     entry.CodecID,
				IsDefault: entry.FlagDefault == 1,
				IsForced:  entry.FlagForced == 1,
				Language:  &entry.Language,
				Channels:  uint32(entry.Audio.Channels),
			}
			audios = append(audios, *a)
			audioIndex += 1
		}
		//
		// Subtitles
		//
		if entry.TrackType == matroska.TrackTypeSubtitle {
			subExt := guessSubtitleExt(entry.CodecID)
			var link *string
			if subExt != "" {
				subExt = subExt[1:] // remove the dot
				x := fmt.Sprintf("%s/subtitle/%d.%s", e.route, entry.TrackNumber, subExt)
				link = &x
			}
			s := &Subtitle{
				Index:     uint32(subtitleIndex),
				Title:     entry.Name,
				Codec:     entry.CodecID,
				IsDefault: entry.FlagDefault == 1,
				IsForced:  entry.FlagForced == 1,
				Extension: &subExt,
				Language:  &entry.Language,
				Link:      link,
			}
			subtitles = append(subtitles, *s)
			subtitleIndex += 1
		}
	}

	mi.Videos = videos
	if len(videos) > 0 {
		mi.Video = &videos[0]
	}
	mi.Audios = audios
	mi.Subtitles = subtitles

	// Close file
	file.Close()

	//
	// Get chapters & fonts \/
	//

	chapters := make([]Chapter, 0)
	fonts := make([]string, 0)

	// Reopen file
	file, err = os.Open(e.path)
	if err != nil {
		return nil, err
	}

	d := ebml.NewDecoder(file) // Use ebml decoder since scanner doesn't have the fields

	_, err = d.DecodeHeader()
	if err != nil {
		return nil, err
	}

	var b matroska.Segment
	if err = d.DecodeBody(&b); err != nil && err != io.EOF {
		return nil, err
	}
	if b.Attachments != nil {
		for _, a := range b.Attachments.AttachedFile {
			if strings.Contains(a.FileMediaType, "font") {
				fonts = append(fonts, a.FileName)
			}
		}
	}
	if b.Chapters != nil {
		for _, c := range b.Chapters.EditionEntry {
			chs := c.ChapterAtom

			for _, ch := range chs { // Go through chapter atoms
				startTime := float32(ch.ChapterTimeStart) / 1e9
				chapter := Chapter{
					StartTime: startTime,
					EndTime:   0, // We don't have the end time, this will be set by the next chapter
				}
				// Get chapter name
				if ch.ChapterDisplay != nil {
					for _, d := range ch.ChapterDisplay {
						if d.ChapString != "" {
							chapter.Name = d.ChapString
						}
					}
				}
				chapters = append(chapters, chapter)
			}
		}
	}
	// Set end time for chapters
	for i := 0; i <= len(chapters)-1; i++ {
		if i == len(chapters)-1 { // Last chapter
			chapters[i].EndTime = float32(duration)
			break
		}
		chapters[i].EndTime = chapters[i+1].StartTime - 1 // Set end time to the start time of the next chapter - 1
	}

	mi.Chapters = chapters
	mi.Fonts = fonts

	return mi, nil
}

func guessSubtitleExt(codecID string) string {
	switch codecID {
	// Audio
	case matroska.AudioCodecAAC:
		return ".aac"
	case matroska.AudioCodecAC3:
		return ".ac3"
	case matroska.AudioCodecMP3:
		return ".mp3"
	// Video
	case matroska.VideoCodecMSCOMP:
		return ".avi"
	// Subtitle
	case matroska.SubtitleCodecTEXTASS:
		return ".ass"
	case matroska.SubtitleCodecTEXTSSA:
		return ".ssa"
	case matroska.SubtitleCodecTEXTUTF8, matroska.SubtitleCodecTEXTASCII:
		return ".srt"
	case matroska.SubtitleCodecVOBSUB, matroska.SubtitleCodecVOBSUBZLIB:
		return ".idx"
	case matroska.SubtitleCodecTEXTWEBVTT:
		return ".vtt"
	default:
		return ""
	}
}
