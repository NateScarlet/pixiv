package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFetchNovel(t *testing.T) {
	getCredential(t)
	LoginFromEnv()
	i := Novel{ID: "11983096"}
	err := i.Fetch()
	assert.NoError(t, err)
	t.Log(i)
	assert.Equal(t, "11983096", i.ID)
	assert.Equal(t, "転生したら獪岳になってんだが俺はこいつのことをよく知らない・玖", i.Title)
	assert.Len(t, i.Tags, 6)
	assert.NotEmpty(t, i.Tags)
	created, err := time.Parse(time.RFC3339, "2019-11-21T16:49:02+00:00")
	assert.NoError(t, err)
	assert.Equal(t, created, i.Created)
	assert.Equal(t, "41540476", i.Author.ID)
	assert.Equal(t, "千晴", i.Author.Name)
	assert.NotEmpty(t, i.Content)
	assert.Equal(t, int64(6), i.PageCount)
	assert.GreaterOrEqual(t, i.CommentCount, int64(72))
	assert.GreaterOrEqual(t, i.LikeCount, int64(3178))
	assert.GreaterOrEqual(t, i.ViewCount, int64(21955))
	assert.GreaterOrEqual(t, i.BookmarkCount, int64(3690))
}
