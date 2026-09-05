package novel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSearchNovel(t *testing.T) {
	var ctx = context.Background()
	result, err := Search(ctx, "パチュリー・ノーレッジ")
	require.NoError(t, err)
	data := result.JSON
	t.Log(data.Raw)
	assert.Equal(t, false, data.Get("error").Bool())
	assert.False(t, data.Get("message").Exists())
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

func TestSearchResultNovelsFillsDescription(t *testing.T) {
	// 回归测试:搜索条目描述字段名为小写 description(旧代码曾误用大写 Description)。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"body":{"novel":{"data":[{"id":"1","title":"t","description":"desc","userId":"2","userName":"a","textCount":10,"bookmarkCount":1,"tags":["tag"],"seriesId":"3","seriesTitle":"s"}]}}}`)
	}))
	defer server.Close()

	c := new(client.Client)
	c.ServerURL = server.URL
	ctx := client.With(context.Background(), c)

	result, err := Search(ctx, "検索語")
	require.NoError(t, err)
	novels := result.Novels()
	require.Len(t, novels, 1)
	assert.Equal(t, "desc", novels[0].Description)
}
