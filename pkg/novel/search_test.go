package novel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestSearchNovel(t *testing.T) {
	var ctx = context.Background()
	result, err := Search(ctx, "パチュリー・ノーレッジ")
	assert.NoError(t, err)
	data := result.JSON
	t.Log(data.Raw)
	assert.Equal(t, false, data.Get("error").Bool())
	assert.Empty(t, data.Get("message"))
	result.ForEach(func(key, value gjson.Result) bool {
		assert.NotEmpty(t, value.Get("id"))
		return true
	})
	novels := result.Novels()
	assert.GreaterOrEqual(t, len(novels), 21)
	for _, i := range novels {
		assert.NotEmpty(t, i.ID)
		assert.NotEmpty(t, i.Tags)
		assert.NotEmpty(t, i.TextCount)
		assert.NotEmpty(t, i.Author.ID)
	}

}
