package artwork

import (
	"context"
	"os"
	"testing"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSearchArtwork(t *testing.T) {
	var ctx = context.Background()
	result, err := Search(ctx, "パチュリー・ノーレッジ")
	require.NoError(t, err)
	data := result.JSON
	// t.Log(data.Raw)
	assert.Equal(t, false, data.Get("error").Bool())
	assert.Empty(t, data.Get("message"))
	dataCount := int(data.Get("illustManga.data.#").Int())
	assert.GreaterOrEqual(t, dataCount, 40)
	count := 0
	result.ForEach(func(key, value gjson.Result) bool {
		count++
		// t.Log(key, value)
		assert.NotEmpty(t, value.Get("id"))
		return true
	})
	assert.GreaterOrEqual(t, count, 40)
	assert.LessOrEqual(t, count, dataCount)
	artworks := result.Artworks()
	assert.Len(t, artworks, count)
	for _, i := range artworks {
		assert.NotEmpty(t, i.ID)
		assert.NotEmpty(t, i.Title)
		assert.NotEmpty(t, i.Type)
		assert.NotEmpty(t, i.Image.Thumb)
		assert.NotEmpty(t, i.Tags)
	}
}

func TestSearchR18Artwork(t *testing.T) {
	if os.Getenv("PIXIV_PHPSESSID") == "" {
		t.Skip()
		return
	}
	var c = new(client.Client)
	c.SetPHPSESSID(os.Getenv("PIXIV_PHPSESSID"))
	c.SetDefaultHeader("User-Agent", client.DefaultUserAgent)

	ctx := client.With(context.Background(), c)
	result, err := Search(
		ctx,
		"パチュリー・ノーレッジ",
		SearchOptionPage(2),
		SearchOptionContentRating(ContentRatingR18),
		SearchOptionOrder(OrderDateDSC),
	)
	require.NoError(t, err)
	artworks := result.Artworks()
	assert.NotEmpty(t, artworks)
	for _, i := range artworks {
		var found bool
		for _, v := range i.Tags {
			if v != "R-18" && v != "R-18G" {
				continue
			}
			found = true
		}
		assert.True(t, found)
	}
}
