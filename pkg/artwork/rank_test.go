package artwork

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankURL(t *testing.T) {
	var ctx = context.Background()
	date, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00+00:00")
	require.NoError(t, err)
	assert.Equal(t, "https://www.pixiv.net/ranking.php", Rank{Mode: "daily"}.URL(ctx).String())
	assert.Equal(t, "https://www.pixiv.net/ranking.php?mode=weekly", Rank{Mode: "weekly"}.URL(ctx).String())
	assert.Equal(t, "https://www.pixiv.net/ranking.php?date=20200101&mode=weekly", Rank{Mode: "weekly", Date: date}.URL(ctx).String())
	assert.Equal(t, "https://www.pixiv.net/ranking.php?content=manga&date=20200101&mode=weekly", Rank{Mode: "weekly", Content: "manga", Date: date}.URL(ctx).String())
}

func TestArtworkRankSimple(t *testing.T) {
	date, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00+00:00")
	require.NoError(t, err)
	rank := &Rank{
		Mode: "daily",
		Date: date,
	}
	err = rank.Fetch(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rank.Items), 48)
	for _, item := range rank.Items {
		assert.NotEmpty(t, item.Rank)
		assert.NotEmpty(t, item.Artwork.ID)
		assert.NotEmpty(t, item.Artwork.Title)
		assert.NotEmpty(t, item.Artwork.PageCount)
		assert.NotEmpty(t, item.Artwork.Image.Regular)
		assert.NotEmpty(t, item.Artwork.Author.ID)
		assert.NotEmpty(t, item.Artwork.Author.Name)
		assert.NotEmpty(t, item.Artwork.Author.Avatar.Mini)
	}
}
