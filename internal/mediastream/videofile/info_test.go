package videofile

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/seanime-app/seanime/internal/util"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMediaInfoExtractor_GetInfo(t *testing.T) {

	//filep := "E:/ANIME/[Judas] Blue Lock (Season 1) [1080p][HEVC x265 10bit][Dual-Audio][Multi-Subs]/[Judas] Blue Lock - S01E03v2.mkv"
	filep := "E:/COLLECTION/One Piece/[Erai-raws] One Piece - 1072 [1080p][Multiple Subtitle][51CB925F].mkv"

	me, err := NewMediaInfoExtractor(filep, util.NewLogger())

	if assert.NoError(t, err) {

		info, err := me.GetInfo(t.TempDir())
		if assert.NoError(t, err) {

			spew.Dump(info)

		}

	}

}
