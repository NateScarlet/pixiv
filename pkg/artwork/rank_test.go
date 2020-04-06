package artwork

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestArtworkRankSimple(t *testing.T) {
	date, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00+00:00")
	assert.NoError(t, err)
	rank := &Rank{
		Mode: "daily",
		Date: date,
	}
	err = rank.Fetch()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(rank.Items), 49)
	for _, item := range rank.Items {
		assert.NotEmpty(t, item.Rank)
		assert.NotEmpty(t, item.Artwork.ID)
		assert.NotEmpty(t, item.Artwork.Title)
		assert.NotEmpty(t, item.Artwork.PageCount)
		assert.NotEmpty(t, item.Artwork.Image.Regular)
		assert.NotEmpty(t, item.Artwork.Author.ID)
		assert.NotEmpty(t, item.Artwork.Author.Name)
		assert.NotEmpty(t, item.Artwork.Author.AvatarURLs.Mini)
	}
}
