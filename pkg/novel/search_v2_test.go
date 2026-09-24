package novel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 小说搜索响应(mock):body 层含 novel.data 数组,条目含已知字段与 seriesId/seriesTitle。
const searchV2Response = `{"body":{"novel":{"data":[
	{"id":"28556784","title":"第９話","description":"desc1","userId":"21083014","userName":"author1","textCount":1000,"bookmarkCount":10,"tags":["tag1","tag2"],"seriesId":"16040720","seriesTitle":"もし、ボーイズバーで働くとして"},
	{"id":"2","title":"第10話","description":"desc2","userId":"3","userName":"author2","textCount":200,"bookmarkCount":2,"tags":[],"seriesId":"16040720","seriesTitle":"もし、ボーイズバーで働くとして"}
]}}}`

func searchV2PayloadFromMock(t *testing.T, body string) SearchPayload {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	p, err := SearchV2(ctx, "検索語")
	require.NoError(t, err)
	return p
}

func TestSearchV2ReturnsImmutablePayload(t *testing.T) {
	p := searchV2PayloadFromMock(t, searchV2Response)
	assert.NotEmpty(t, p.Raw())

	var items []ItemInSearchPayload
	for item := range p.Items() {
		items = append(items, item)
	}
	require.Len(t, items, 2)

	i := items[0]
	assert.Equal(t, "28556784", i.ID())
	assert.Equal(t, "第９話", i.Title())
	assert.Equal(t, "desc1", i.Description())
	assert.Equal(t, "21083014", i.AuthorID())
	assert.Equal(t, "author1", i.AuthorName())
	assert.Equal(t, int64(1000), i.TextCount())
	assert.Equal(t, int64(10), i.BookmarkCount())
	assert.Equal(t, []string{"tag1", "tag2"}, collectSearchTags(i))
	assert.Equal(t, "16040720", i.SeriesID())
	assert.Equal(t, "もし、ボーイズバーで働くとして", i.SeriesTitle())
	assert.NotEmpty(t, i.Raw())
}

func collectSearchTags(i ItemInSearchPayload) []string {
	var ret []string
	for tag := range i.Tags() {
		ret = append(ret, tag)
	}
	return ret
}

func TestSearchV2RequiresQuery(t *testing.T) {
	_, err := SearchV2(context.Background(), "")
	require.Error(t, err)
}

// 搜索接口被边缘节点以 403 拒绝时，错误应说明状态码，
// 而不是把整页 HTML 当作响应体去解析后报 invalid json。
func TestSearchV2ShouldReportRejectedStatus(t *testing.T) {
	const htmlForbiddenBody = "<html>\r\n<head><title>403 Forbidden</title></head>\r\n" +
		"<body>\r\n<center><h1>403 Forbidden</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, htmlForbiddenBody)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	_, err := SearchV2(ctx, "検索語")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.NotContains(t, err.Error(), "<html>")
}

// TestSearchV2Live 验证真实匿名接口的搜索响应解析(含字段名与 isAdContainer 广告条目)。
func TestSearchV2Live(t *testing.T) {
	testenv.RequireLive(t)
	p, err := SearchV2(context.Background(), "パチュリー・ノーレッジ")
	require.NoError(t, err)
	var n int
	for item := range p.Items() {
		require.NotEmpty(t, item.ID())
		if n == 0 {
			// 记录首条原始 JSON,便于确认字段名(description vs Description)。
			t.Logf("first item raw: %.300s", string(item.Raw()))
		}
		n++
		if n >= 3 {
			break
		}
	}
	assert.GreaterOrEqual(t, n, 1)
}
