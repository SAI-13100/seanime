package image_downloader

import (
	"github.com/seanime-app/seanime/internal/util"
	"testing"
	"time"
)

func TestImageDownloader_DownloadImages(t *testing.T) {

	tests := []struct {
		name        string
		urls        []string
		downloadDir string
		cancelAfter int
	}{
		{
			name: "test1",
			urls: []string{"https://s4.anilist.co/file/anilistcdn/media/anime/banner/153518-7uRvV7SLqmHV.jpg",
				"https://s4.anilist.co/file/anilistcdn/media/anime/cover/medium/bx153518-LEK6pAXtI03D.jpg"},
			downloadDir: "./test1",
			cancelAfter: 0,
		},
		{
			name:        "test1",
			urls:        []string{"https://s4.anilist.co/file/anilistcdn/media/anime/banner/153518-7uRvV7SLqmHVn.jpg"},
			downloadDir: "./test1",
			cancelAfter: 0,
		},
	}

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {

			id := NewImageDownloader(tt.downloadDir, util.NewLogger())

			if tt.cancelAfter > 0 {
				go func() {
					time.Sleep(time.Duration(tt.cancelAfter) * time.Second)
					close(id.cancelChannel)
				}()
			}

			if err := id.DownloadImages(tt.urls); err != nil {
				t.Errorf("ImageDownloader.DownloadImages() error = %v", err)
			}

			imgPath, ok := id.GetImageFilenameByUrl(tt.urls[0])
			if !ok {
				t.Errorf("ImageDownloader.GetImagePathByUrl() error")
			} else {
				t.Logf("ImageDownloader.GetImagePathByUrl() = %v", imgPath)
			}

		})

	}

	time.Sleep(1 * time.Second)
}
