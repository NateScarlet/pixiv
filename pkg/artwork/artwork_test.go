package artwork

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchArtwork(t *testing.T) {
	i := Artwork{ID: "22238487"}
	err := i.Fetch()
	t.Log(i)
	require.NoError(t, err)
	assert.Equal(t, "22238487", i.ID)
	assert.Equal(t, "無題", i.Title)
	assert.Len(t, i.Tags, 2)
	assert.Equal(t, []string{"東方", "パチュリー・ノーレッジ"}, i.Tags)
	created, _ := time.Parse(time.RFC3339, "2011-10-07T17:22:58+00:00")
	assert.Equal(t, created, i.Created)
	assert.Equal(t, "789096", i.Author.ID)
	assert.Equal(t, "CHN^NateScarlet", i.Author.Name)
	assert.Equal(t, int64(1), i.PageCount)
	assert.LessOrEqual(t, int64(4), i.CommentCount)
	assert.LessOrEqual(t, int64(54), i.LikeCount)
	assert.LessOrEqual(t, int64(899), i.ViewCount)
	assert.LessOrEqual(t, int64(12), i.BookmarkCount)
}

func TestFetchPages(t *testing.T) {
	i := Artwork{ID: "52200823"}
	err := i.FetchPages()
	require.NoError(t, err)
	t.Log(i)
	assert.Equal(t, "52200823", i.ID)
	assert.Len(t, i.Pages, 3)
	for _, i := range i.Pages {
		assert.NotEmpty(t, i.URLs.Mini)
		assert.NotEmpty(t, i.URLs.Thumb)
		assert.NotEmpty(t, i.URLs.Small)
		assert.NotEmpty(t, i.URLs.Regular)
		assert.NotEmpty(t, i.URLs.Original)
		assert.NotEmpty(t, i.Width)
		assert.NotEmpty(t, i.Height)
	}

}
