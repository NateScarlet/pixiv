package artwork

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

// 有系列的画作详情响应(mock),基于 issue #84 的样本(artwork 147500314)。
const fetchWithSeriesResponse = `{"body":{"illustId":"147500314","illustTitle":"【ポケダン漫画】ユメシルベ 第1話","illustType":1,"seriesNavData":{"seriesType":"manga","seriesId":349076,"title":"【ポケダン漫画】ユメシルベ","order":1,"prev":null,"next":null},"aiType":0}}`

// 无系列的画作详情响应:不包含 seriesNavData 字段。
const fetchWithoutSeriesResponse = `{"body":{"illustId":"100","illustTitle":"no series","illustType":0,"aiType":0}}`

func fetchPayloadFromMock(t *testing.T, body string) FetchPayload {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	p, err := Fetch(ctx, "147500314")
	require.NoError(t, err)
	return p
}

func TestFetchPayloadSeries(t *testing.T) {
	p := fetchPayloadFromMock(t, fetchWithSeriesResponse)
	s := p.Series()
	require.False(t, s.IsZero())
	assert.Equal(t, "349076", s.ID())
	assert.Equal(t, "【ポケダン漫画】ユメシルベ", s.Title())
	assert.Equal(t, int64(1), s.Order())
	// prev/next 为 null 时相邻章节记录为零值。
	assert.Equal(t, "", s.Prev().ID())
	assert.Equal(t, "", s.Next().ID())
	assert.NotEmpty(t, s.Raw())
}

func TestFetchPayloadSeriesWithNeighbors(t *testing.T) {
	body := `{"body":{"illustId":"1","illustTitle":"t","seriesNavData":{"seriesType":"manga","seriesId":349076,"title":"T","order":11,"prev":{"title":"第９話","order":10,"id":"28556784","available":true},"next":{"title":"第11話","order":12,"id":"999","available":true}}}}`
	p := fetchPayloadFromMock(t, body)
	s := p.Series()
	assert.Equal(t, int64(11), s.Order())
	assert.Equal(t, "28556784", s.Prev().ID())
	assert.Equal(t, "第９話", s.Prev().Title())
	assert.Equal(t, int64(10), s.Prev().Order())
	assert.True(t, s.Prev().Available())
	assert.Equal(t, "999", s.Next().ID())
	assert.Equal(t, int64(12), s.Next().Order())
}

func TestFetchPayloadWithoutSeriesReturnsZeroSeries(t *testing.T) {
	p := fetchPayloadFromMock(t, fetchWithoutSeriesResponse)
	s := p.Series()
	assert.True(t, s.IsZero())
	assert.Equal(t, "", s.ID())
	assert.Equal(t, "", s.Title())
	assert.Equal(t, int64(0), s.Order())
}

// TestFetchPayloadSeriesLive 验证真实匿名接口解析 seriesNavData(issue #84 样本: 147500314)。
// 匿名接口无需登录;需要网络可用并配合代理访问 pixiv。
func TestFetchPayloadSeriesLive(t *testing.T) {
	testenv.RequireLive(t)
	p, err := Fetch(context.Background(), "147500314")
	require.NoError(t, err)
	s := p.Series()
	require.False(t, s.IsZero())
	assert.Equal(t, "349076", s.ID())
	assert.Equal(t, "【ポケダン漫画】ユメシルベ", s.Title())
	assert.Equal(t, int64(1), s.Order())
}
