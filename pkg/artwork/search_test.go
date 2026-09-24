package artwork

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchV2Response 画作搜索响应(mock):body 层含 illustManga.data 数组。
const searchV2Response = `{"body":{"illustManga":{"data":[
	{"id":"147500314","title":"【ポケダン漫画】ユメシルベ 第1話","illustType":1,"url":"https://i.pximg.net/thumb.jpg","userId":"1","userName":"author","pageCount":1,"tags":["tag1"]},
	{"id":"2","title":"second","illustType":0,"userId":"2","userName":"author2","pageCount":3,"tags":[]}
]}}}`

// htmlForbiddenBody 是边缘节点拒绝请求时返回的 HTML 错误页。
const htmlForbiddenBody = "<html>\r\n<head><title>403 Forbidden</title></head>\r\n" +
	"<body>\r\n<center><h1>403 Forbidden</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n"

// searchV2FromServer 起一个固定返回 status/body 的服务端，并对它执行一次画作搜索。
func searchV2FromServer(t *testing.T, status int, body string) (SearchPayload, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	return SearchV2(client.With(context.Background(), c), "検索語")
}

func TestSearchV2PayloadItems(t *testing.T) {
	p, err := searchV2FromServer(t, http.StatusOK, searchV2Response)
	require.NoError(t, err)
	assert.NotEmpty(t, p.Raw())

	var items []ItemInSearchPayload
	for item := range p.Items() {
		items = append(items, item)
	}
	require.Len(t, items, 2)

	assert.Equal(t, "147500314", items[0].ID())
	assert.Equal(t, "【ポケダン漫画】ユメシルベ 第1話", items[0].Title())
	assert.Equal(t, "1", items[0].AuthorID())
	assert.Equal(t, 1, items[0].PageCount())
}

// 搜索接口被边缘节点以 403 拒绝时，错误应说明状态码，
// 而不是把整页 HTML 当作响应体去解析后报 invalid json。
func TestSearchV2ShouldReportRejectedStatus(t *testing.T) {
	_, err := searchV2FromServer(t, http.StatusForbidden, htmlForbiddenBody)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.NotContains(t, err.Error(), "<html>")
}
