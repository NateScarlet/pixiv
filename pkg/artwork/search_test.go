package artwork

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestSearchArtwork(t *testing.T) {
	var ctx = context.Background()
	result, err := Search(ctx, "パチュリー・ノーレッジ")
	assert.NoError(t, err)
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

	R18result, _ := Search(ctx, "パチュリー・ノーレッジ", SearchOptionPage(2), SearchOptionMode("r18"), SearchOptionOrder("date"))
	r18 := R18result.Artworks()
	for _, i := range r18 {
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
