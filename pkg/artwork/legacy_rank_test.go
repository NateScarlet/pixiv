package artwork

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankURL(t *testing.T) {
	var ctx = context.Background()
	date, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00+00:00")
	require.NoError(t, err)
	for _, tt := range []struct {
		name string
		rank Rank
		want string
	}{
		{"默认", Rank{Mode: "daily"}, "https://www.pixiv.net/ranking.php"},
		{"模式", Rank{Mode: "weekly"}, "https://www.pixiv.net/ranking.php?mode=weekly"},
		{"日期", Rank{Mode: "weekly", Date: date}, "https://www.pixiv.net/ranking.php?date=20200101&mode=weekly"},
		{"内容与日期", Rank{Mode: "weekly", Content: "manga", Date: date}, "https://www.pixiv.net/ranking.php?content=manga&date=20200101&mode=weekly"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.rank.URL(ctx)
			assert.Equal(t, tt.want, u.String())
		})
	}
}

// issue #104: ranking.php 不校验状态码时，边缘节点的 403 错误页会被解析成
// 「零条目榜单」并报成 no rank items found，调用方拿不到状态码，无法退避重试。
func TestRankFetchShouldRejectNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "<html><head><title>403 Forbidden</title></head><body><center><h1>403 Forbidden</h1></center><hr><center>nginx</center></body></html>")
	}))
	t.Cleanup(server.Close)
	ctx := client.With(context.Background(), client.New(client.WithServerURL(server.URL)))

	rank := &Rank{Mode: "daily"}
	err := rank.Fetch(ctx)
	require.Error(t, err)
	var rej *client.ErrAPIRejected
	require.ErrorAs(t, err, &rej)
	require.NotNil(t, rej.Response)
	assert.Equal(t, http.StatusForbidden, rej.Response.StatusCode)
	assert.Empty(t, rank.Items)
}

func TestArtworkRankSimple(t *testing.T) {
	testenv.RequireLive(t)
	date, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00+00:00")
	require.NoError(t, err)
	rank := &Rank{
		Mode: "daily",
		Date: date,
	}
	err = rank.Fetch(context.Background())
	require.NoError(t, err)
	// 历史榜单的条目数与字段随 pixiv 侧变化（2026-09 实测返回 44 条，
	// 已删除作品标题为空、页数字段缺省），因此只断言基本解析成功，
	// 不再对条目数与完整字段做断言。
	assert.NotEmpty(t, rank.Items)
	for _, item := range rank.Items {
		assert.NotEmpty(t, item.Rank)
		assert.NotEmpty(t, item.Artwork.Image.Regular)
	}
}
