package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// htmlForbiddenBody 是边缘节点拒绝请求时返回的 HTML 错误页。
//
// 实测该页面的 Content-Type 为 text/html，因此不能靠它判断该不该解析 JSON，
// 状态码才是唯一的信号。
const htmlForbiddenBody = "<html>\r\n<head><title>403 Forbidden</title></head>\r\n" +
	"<body>\r\n<center><h1>403 Forbidden</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n"

// newResponseServer 起一个固定返回 status 与 body 的服务端，返回指向它的客户端。
func newResponseServer(t *testing.T, status int, contentType, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return New(WithServerURL(srv.URL))
}

// doGet 用 c 发起一次请求，返回响应与错误。
func doGet(t *testing.T, c *Client) (*http.Response, error) {
	t.Helper()
	return c.GetWithContext(t.Context(), c.EndpointURL("/ajax/search/artworks/test", nil).String())
}

func TestParseAPIResponseV2ShouldRejectNonOKStatus(t *testing.T) {
	c := newResponseServer(t, http.StatusForbidden, "text/html; charset=utf-8", htmlForbiddenBody)
	resp, err := doGet(t, c)
	require.NoError(t, err)
	defer resp.Body.Close()

	_, err = ParseAPIResponseV2(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	// 状态码是失败原因，不该把整页 HTML 混进错误。
	assert.NotContains(t, err.Error(), "<html>")
}

func TestParseAPIResponseV2ShouldRejectUnparsableBody(t *testing.T) {
	c := newResponseServer(t, http.StatusOK, "text/html; charset=utf-8", htmlForbiddenBody)
	resp, err := doGet(t, c)
	require.NoError(t, err)
	defer resp.Body.Close()

	_, err = ParseAPIResponseV2(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid json")
}

func TestParseAPIResponseV2ShouldReturnBody(t *testing.T) {
	c := newResponseServer(t, http.StatusOK, "application/json", `{"error":false,"body":{"x":1}}`)
	resp, err := doGet(t, c)
	require.NoError(t, err)
	defer resp.Body.Close()

	raw, err := ParseAPIResponseV2(resp)
	require.NoError(t, err)
	assert.JSONEq(t, `{"x":1}`, string(raw))
}

func TestParseAPIResponseV2ShouldReportAPIError(t *testing.T) {
	c := newResponseServer(t, http.StatusOK, "application/json", `{"error":true,"message":"未登录"}`)
	resp, err := doGet(t, c)
	require.NoError(t, err)
	defer resp.Body.Close()

	_, err = ParseAPIResponseV2(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未登录")
}

// 成功状态码不只有 200：带正文的 2xx 都应视为成功，
// 而定义里没有正文的 204 与重定向、错误状态一样不该进入解析。
func TestParseAPIResponseV2ShouldAcceptBodylessSuccessStatusAsEmpty(t *testing.T) {
	c := newResponseServer(t, http.StatusNoContent, "", "")
	resp, err := doGet(t, c)
	require.NoError(t, err)
	defer resp.Body.Close()

	raw, err := ParseAPIResponseV2(resp)
	require.NoError(t, err)
	assert.True(t, len(raw) == 0 || strings.TrimSpace(string(raw)) == "")
}
