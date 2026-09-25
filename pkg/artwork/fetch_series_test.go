package artwork

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 作品系列页面响应(mock):基于真实样本 series 66844 第 1 页(2026-09),
// illustSeries 含请求的系列与另一个同作者系列(验证按 id 匹配),
// thumbnails.illust 含章节缩略图与一个不属于本页章节的多余条目,
// 文本字段为可断言的占位值,字段结构与真实响应一致。
const fetchSeriesResponse = `{"error":false,"message":"","body":{"illustSeries":[{"id":"66844","title":"series 66844","total":250,"firstIllustId":"77481424","latestIllustId":"149834895","url":"https://i.pximg.net/c/782x410_80_a2_g5/illust-series-cover-original/img/2019/11/30/11/04/22/Z3z9sLkRkzTSnDe3MWt9aQHPKw1Lh1o8.jpg"},{"id":"287076","title":"series 287076","total":8,"firstIllustId":"131006042","latestIllustId":"148126612","url":"https://i.pximg.net/c/782x410_80_a2_g2/img-master/img/2026/08/07/19/19/02/148126612_p0_master1200.jpg"}],"page":{"series":[{"workId":"149834895","order":250},{"workId":"149292642","order":249},{"workId":"148730398","order":248}],"seriesId":66844,"total":250},"thumbnails":{"illust":[{"id":"149834895","title":"chapter 149834895","url":"https://i.pximg.net/c/250x250_80_a2/custom-thumb/img/2026/09/19/11/00/06/149834895_p0_custom1200.jpg","userId":"14499092","userName":"author name","seriesId":"66844","seriesTitle":"series 66844"},{"id":"149292642","title":"chapter 149292642","url":"https://i.pximg.net/c/250x250_80_a2/custom-thumb/img/2026/09/05/11/13/57/149292642_p0_custom1200.jpg","userId":"14499092","userName":"author name","seriesId":"66844","seriesTitle":"series 66844"},{"id":"148730398","title":"chapter 148730398","url":"https://i.pximg.net/c/250x250_80_a2/custom-thumb/img/2026/08/22/11/00/05/148730398_p0_custom1200.jpg","userId":"14499092","userName":"author name","seriesId":"66844","seriesTitle":"series 66844"},{"id":"149971144","title":"chapter 149971144","url":"https://i.pximg.net/c/250x250_80_a2/custom-thumb/img/2026/09/22/16/41/57/149971144_p0_custom1200.jpg","userId":"14499092","userName":"author name","seriesId":null,"seriesTitle":null}]}}}`

// 无系列元数据的页面响应:illustSeries 缺失,章节列表为空。
const fetchSeriesEmptyResponse = `{"error":false,"message":"","body":{"page":{"series":[],"seriesId":0,"total":0},"thumbnails":{"illust":[]}}}`

// fetchSeriesPayloadFromMock 起一个只回固定响应体的 mock 端点,
// 返回解析结果与请求到的查询串。
func fetchSeriesPayloadFromMock(t *testing.T, body string, opts ...FetchSeriesOption) (FetchSeriesPayload, string) {
	t.Helper()
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	p, err := FetchSeries(ctx, "66844", opts...)
	require.NoError(t, err)
	return p, gotQuery
}

func TestFetchSeriesPayloadMetadata(t *testing.T) {
	p, _ := fetchSeriesPayloadFromMock(t, fetchSeriesResponse)
	assert.Equal(t, "66844", p.ID())
	assert.NotEmpty(t, p.Raw())
	// 元数据按请求的系列 id 匹配,而不是取 illustSeries 的第一个元素。
	assert.Equal(t, "series 66844", p.Title())
	assert.Equal(t, int64(250), p.Total())
	assert.Equal(t, "77481424", p.FirstIllustID())
	assert.Equal(t, "149834895", p.LatestIllustID())
	assert.Equal(t, "https://i.pximg.net/c/782x410_80_a2_g5/illust-series-cover-original/img/2019/11/30/11/04/22/Z3z9sLkRkzTSnDe3MWt9aQHPKw1Lh1o8.jpg", p.CoverURL())
}

func TestFetchSeriesPayloadChapters(t *testing.T) {
	p, _ := fetchSeriesPayloadFromMock(t, fetchSeriesResponse)
	var chapters []ChapterInFetchSeriesPayload
	for c := range p.Chapters() {
		chapters = append(chapters, c)
	}
	require.Len(t, chapters, 3)

	assert.Equal(t, "149834895", chapters[0].ID())
	assert.Equal(t, int64(250), chapters[0].Order())
	// 章节缩略图按作品 id 关联,不依赖数组下标对齐。
	assert.Equal(t, "chapter 149834895", chapters[0].Title())
	assert.Equal(t, "https://i.pximg.net/c/250x250_80_a2/custom-thumb/img/2026/09/19/11/00/06/149834895_p0_custom1200.jpg", chapters[0].URL())
	assert.Equal(t, "14499092", chapters[0].AuthorID())
	assert.Equal(t, "author name", chapters[0].AuthorName())

	assert.Equal(t, "149292642", chapters[1].ID())
	assert.Equal(t, int64(249), chapters[1].Order())
	assert.Equal(t, "chapter 149292642", chapters[1].Title())

	assert.Equal(t, "148730398", chapters[2].ID())
	assert.Equal(t, int64(248), chapters[2].Order())
}

func TestFetchSeriesRequestPage(t *testing.T) {
	// 服务端要求 p 参数(缺失会 400),默认请求第 1 页。
	_, q := fetchSeriesPayloadFromMock(t, fetchSeriesResponse)
	assert.Equal(t, "p=1", q)

	_, q = fetchSeriesPayloadFromMock(t, fetchSeriesResponse, FetchSeriesWithPage(2))
	assert.Equal(t, "p=2", q)
}

func TestFetchSeriesPayloadWithoutMetadata(t *testing.T) {
	p, _ := fetchSeriesPayloadFromMock(t, fetchSeriesEmptyResponse)
	// 缺失字段返回零值而不是报错。
	assert.Equal(t, "", p.Title())
	assert.Equal(t, int64(0), p.Total())
	assert.Equal(t, "", p.FirstIllustID())
	assert.Equal(t, "", p.LatestIllustID())
	assert.Equal(t, "", p.CoverURL())
	assert.Empty(t, slices.Collect(p.Chapters()))
}

func TestFetchSeriesRequiresID(t *testing.T) {
	c := client.New(client.WithServerURL("http://127.0.0.1:0"))
	ctx := client.With(context.Background(), c)
	_, err := FetchSeries(ctx, "")
	require.Error(t, err)
}

// 有章节但缩略图缺失的响应:缩略图来源的访问器应返回零值。
const fetchSeriesChapterWithoutThumbnailResponse = `{"error":false,"message":"","body":{"page":{"series":[{"workId":"1","order":7}]}}}`

func TestFetchSeriesChapterWithoutThumbnail(t *testing.T) {
	p, _ := fetchSeriesPayloadFromMock(t, fetchSeriesChapterWithoutThumbnailResponse)
	chapters := slices.Collect(p.Chapters())
	require.Len(t, chapters, 1)
	c := chapters[0]
	// 章节自身的字段照常可读。
	assert.Equal(t, "1", c.ID())
	assert.Equal(t, int64(7), c.Order())
	// 缩略图缺失时相关访问器返回零值而不是报错。
	assert.Equal(t, "", c.Title())
	assert.Equal(t, "", c.URL())
	assert.Equal(t, "", c.AuthorID())
	assert.Equal(t, "", c.AuthorName())
}

// TestFetchSeriesLive 验证真实匿名接口(2026-09: series 66844 共 250 章)。
// 匿名接口无需登录;需要网络可用并配合代理访问 pixiv。
func TestFetchSeriesLive(t *testing.T) {
	testenv.RequireLive(t)
	ctx := context.Background()

	p, err := FetchSeries(ctx, "66844")
	require.NoError(t, err)
	assert.Equal(t, int64(250), p.Total())
	assert.Equal(t, "77481424", p.FirstIllustID())
	assert.Equal(t, "149834895", p.LatestIllustID())
	assert.NotEmpty(t, p.Title())

	chapters := slices.Collect(p.Chapters())
	require.NotEmpty(t, chapters)
	assert.Equal(t, int64(250), chapters[0].Order())

	p2, err := FetchSeries(ctx, "66844", FetchSeriesWithPage(2))
	require.NoError(t, err)
	chapters = slices.Collect(p2.Chapters())
	require.NotEmpty(t, chapters)
	assert.Equal(t, int64(238), chapters[0].Order())
}
