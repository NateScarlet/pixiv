package artwork

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFetchRank(t *testing.T) {
	testenv.RequireLive(t)
	payload, err := FetchRank(context.Background(), DailyRank)
	require.NoError(t, err)
	var n int
	for item := range payload.Items() {
		n++
		if gjson.GetBytes(item.Raw(), "mask_reason").String() != "" {
			// 未登录时 pixiv 把不可见作品替换为占位条目，mask_reason 标注原因
			// （例如 login_only）：条目只剩 ID 与占位图，标题与作者名必然为空，
			// 故除存在性外的字段断言对它没有意义。
			continue
		}
		assert.NotEmpty(t, item.ID())
		assert.NotEmpty(t, item.Title())
		assert.NotEmpty(t, item.AuthorID())
		assert.NotEmpty(t, item.AuthorName())
		assert.NotEmpty(t, item.Width())
		assert.NotEmpty(t, item.Height())
	}
	assert.GreaterOrEqual(t, n, 45)
}

// fetchRankWithMock 用给定 status 与 body 起一个假服务端，返回指向它的 context。
func fetchRankWithMock(t *testing.T, status int, contentType, body string) context.Context {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return client.With(context.Background(), client.New(client.WithServerURL(server.URL)))
}

// issue #104: 边缘节点拒绝请求时返回的整页 HTML 不能被当作榜单数据返回给调用方
// (err == nil)，否则 403 / 429 与「响应格式不对」无法区分，也无从退避重试。
func TestFetchRankShouldRejectNon2xx(t *testing.T) {
	ctx := fetchRankWithMock(t, http.StatusForbidden, "text/html; charset=utf-8",
		"<html><head><title>403 Forbidden</title></head><body><center><h1>403 Forbidden</h1></center><hr><center>nginx</center></body></html>")

	_, err := FetchRank(ctx, DailyRank)
	require.Error(t, err)
	// 调用方按状态码退避重试或提示重新登录的入口。
	var rej *client.ErrAPIRejected
	require.ErrorAs(t, err, &rej)
	require.NotNil(t, rej.Response)
	assert.Equal(t, http.StatusForbidden, rej.Response.StatusCode)
}

// ranking.php 是旧式端点，响应没有 {error, body} 信封，contents 在顶层。
// 状态校验不能顺手把它套进信封解析里，否则成功路径会一起被弄坏。
func TestFetchRankShouldReturnTopLevelContents(t *testing.T) {
	ctx := fetchRankWithMock(t, http.StatusOK, "application/json",
		`{"contents":[{"illust_id":"148882180","title":"t","rank":1}],"mode":"daily"}`)

	payload, err := FetchRank(ctx, DailyRank)
	require.NoError(t, err)
	var n int
	for item := range payload.Items() {
		n++
		assert.Equal(t, "148882180", item.ID())
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, http.StatusOK, payload.Response().StatusCode)
}

func TestItemInFetchRankPayloadMaxWidth1200URL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "master thumbnail normalized to regular",
			url:  "https://i.pximg.net/c/480x960/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
			want: "https://i.pximg.net/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
		},
		{
			name: "unrecognized url returned as-is",
			url:  "https://s.pximg.net/common/images/limit_unviewable_s.png",
			want: "https://s.pximg.net/common/images/limit_unviewable_s.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := ItemInFetchRankPayload{json.RawMessage(`{"url":"` + tt.url + `"}`)}
			assert.Equal(t, tt.want, item.MaxWidth1200URL())
		})
	}
}
