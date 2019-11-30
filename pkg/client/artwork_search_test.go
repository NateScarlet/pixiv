package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestSearchArtwork(t *testing.T) {
	result, err := SearchArtwork("パチュリー・ノーレッジ", 1)
	assert.NoError(t, err)
	data := result.JSON
	t.Log(data.Raw)
	assert.Equal(t, false, data.Get("error").Bool())
	assert.Empty(t, data.Get("message"))
	assert.Equal(t, int64(60), data.Get("illustManga.data.#").Int())
	count := 0
	result.ForEach(func(key, value gjson.Result) bool {
		count++
		t.Log(key, value)
		assert.NotEmpty(t, value.Get("illustId"))
		return true
	})
	assert.LessOrEqual(t, 59, count)
	assert.LessOrEqual(t, count, 60)
	artworks := result.Artworks()
	assert.Len(t, artworks, count)
	for _, i := range artworks {
		assert.NotEmpty(t, i.ID)
		assert.NotEmpty(t, i.Tags)
	}
}
