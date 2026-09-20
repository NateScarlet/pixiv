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

func novelFetchPayloadFromMock(t *testing.T, body string) FetchPayload {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	p, err := Fetch(ctx, "28612019")
	require.NoError(t, err)
	return p
}

func TestFetchReturnsImmutablePayloadWithKnownFields(t *testing.T) {
	p := novelFetchPayloadFromMock(t, fetchNovelWithSeriesResponse)
	assert.Equal(t, "28612019", p.ID())
	assert.Equal(t, "もし、ボーイズバーで働くとして", p.Title())
	assert.Equal(t, ManuallyCreated, p.CreationMethod())

	s := p.Series()
	require.False(t, s.IsZero())
	assert.Equal(t, "16040720", s.ID())
	assert.Equal(t, "もし、ボーイズバーで働くとして", s.Title())
	assert.Equal(t, int64(11), s.Order())
	assert.Equal(t, "28556784", s.Prev().ID())
	assert.Equal(t, "第９話", s.Prev().Title())
	assert.Equal(t, int64(10), s.Prev().Order())
	assert.True(t, s.Prev().Available())
	// next 为 null 时相邻章节记录为零值。
	assert.Equal(t, "", s.Next().ID())

	assert.NotEmpty(t, p.Raw())
}

func TestFetchPayloadWithoutSeriesReturnsZeroSeries(t *testing.T) {
	p := novelFetchPayloadFromMock(t, `{"body":{"title":"no series"}}`)
	assert.True(t, p.Series().IsZero())
}

// TestFetchSeriesLive 验证真实匿名接口解析 seriesNavData(issue #84 样本: novel 28612019)。
// 匿名接口无需登录;需要网络可用并配合代理访问 pixiv。
func TestFetchSeriesLive(t *testing.T) {
	testenv.RequireLive(t)
	p, err := Fetch(context.Background(), "28612019")
	require.NoError(t, err)
	s := p.Series()
	require.False(t, s.IsZero())
	assert.Equal(t, "16040720", s.ID())
	assert.Equal(t, "もし、ボーイズバーで働くとして", s.Title())
	assert.Equal(t, int64(11), s.Order())
}
