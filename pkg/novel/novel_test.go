package novel

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchNovel(t *testing.T) {
	if os.Getenv("PIXIV_PHPSESSID") == "" {
		t.Skip("need login")
	}
	i := Novel{ID: "11983096"}
	err := i.Fetch()
	require.NoError(t, err)
	t.Log(i)
	assert.Equal(t, "11983096", i.ID)
	assert.Equal(t, "転生したら獪岳になってんだが俺はこいつのことをよく知らない・玖", i.Title)
	assert.GreaterOrEqual(t, len(i.Tags), 6)
	created, err := time.Parse(time.RFC3339, "2019-11-21T16:49:02+00:00")
	assert.NoError(t, err)
	assert.Equal(t, created, i.Created)
	assert.Equal(t, "41540476", i.Author.ID)
	assert.NotEmpty(t, i.Author.Name)
	assert.NotEmpty(t, i.Content)
	assert.Equal(t, int64(6), i.PageCount)
	assert.GreaterOrEqual(t, i.CommentCount, int64(72))
	assert.GreaterOrEqual(t, i.LikeCount, int64(3178))
	assert.GreaterOrEqual(t, i.ViewCount, int64(21955))
	assert.GreaterOrEqual(t, i.BookmarkCount, int64(3690))
	assert.Equal(t, "https://www.pixiv.net/novel/show.php?id=11983096", i.URL().String())
}
