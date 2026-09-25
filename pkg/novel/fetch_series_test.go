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

// 小说系列元数据响应(mock):基于真实样本 novel series 16040720(2026-09),
// 文本字段为可断言的占位值,字段结构与真实响应一致。
const fetchSeriesResponse = `{"error":false,"message":"","body":{"id":"16040720","title":"series title","caption":"cap","userId":"124440243","userName":"author","total":11,"publishedContentCount":11,"firstNovelId":"28322726","latestNovelId":"28612019","cover":{"urls":{"original":"https://i.pximg.net/novel-cover-original/img/2026/06/12/06/18/13/sci16040720_af8595d47a44dfb7da915aa3847992ec.png"}}}}`

// 空响应体:可选字段全部缺失。
const fetchSeriesEmptyResponse = `{"error":false,"message":"","body":{}}`

func fetchSeriesPayloadFromMock(t *testing.T, body string) FetchSeriesPayload {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	p, err := FetchSeries(ctx, "16040720")
	require.NoError(t, err)
	return p
}

func TestFetchSeriesPayloadMetadata(t *testing.T) {
	p := fetchSeriesPayloadFromMock(t, fetchSeriesResponse)
	assert.Equal(t, "16040720", p.ID())
	assert.NotEmpty(t, p.Raw())
	assert.Equal(t, "series title", p.Title())
	assert.Equal(t, int64(11), p.Total())
	assert.Equal(t, "28322726", p.FirstNovelID())
	assert.Equal(t, "28612019", p.LatestNovelID())
	assert.Equal(t, "https://i.pximg.net/novel-cover-original/img/2026/06/12/06/18/13/sci16040720_af8595d47a44dfb7da915aa3847992ec.png", p.CoverURL())
}

func TestFetchSeriesPayloadWithoutFields(t *testing.T) {
	p := fetchSeriesPayloadFromMock(t, fetchSeriesEmptyResponse)
	// 缺失字段返回零值而不是报错。
	assert.Equal(t, "", p.Title())
	assert.Equal(t, int64(0), p.Total())
	assert.Equal(t, "", p.FirstNovelID())
	assert.Equal(t, "", p.LatestNovelID())
	assert.Equal(t, "", p.CoverURL())
}

func TestFetchSeriesRequiresID(t *testing.T) {
	c := client.New(client.WithServerURL("http://127.0.0.1:0"))
	ctx := client.With(context.Background(), c)
	_, err := FetchSeries(ctx, "")
	require.Error(t, err)
}

// TestFetchSeriesMetadataLive 验证真实匿名接口(2026-09: novel series 16040720 共 11 章)。
// 匿名接口无需登录;需要网络可用并配合代理访问 pixiv。
func TestFetchSeriesMetadataLive(t *testing.T) {
	testenv.RequireLive(t)
	p, err := FetchSeries(context.Background(), "16040720")
	require.NoError(t, err)
	assert.Equal(t, int64(11), p.Total())
	assert.Equal(t, "28322726", p.FirstNovelID())
	assert.Equal(t, "28612019", p.LatestNovelID())
	assert.NotEmpty(t, p.Title())
	assert.NotEmpty(t, p.CoverURL())
}
