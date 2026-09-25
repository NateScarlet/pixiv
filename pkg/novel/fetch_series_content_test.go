package novel

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

// 小说系列章节列表响应(mock):基于真实样本 novel series_content 16040720(2026-09),
// thumbnails.novel 含章节缩略图与一个不属于本页章节的多余条目,
// 文本字段为可断言的占位值,字段结构与真实响应一致。
const fetchSeriesContentResponse = `{"error":false,"message":"","body":{"page":{"seriesContents":[{"id":"28322726","userId":"124440243","title":"content 28322726","series":{"id":16040720,"contentOrder":1}},{"id":"28322750","userId":"124440243","title":"content 28322750","series":{"id":16040720,"contentOrder":2}},{"id":"28322797","userId":"124440243","title":"content 28322797","series":{"id":16040720,"contentOrder":3}}]},"thumbnails":{"novel":[{"id":"28322726","title":"chapter 28322726","url":"https://i.pximg.net/c/600x600/novel-cover-master/img/2026/06/12/06/18/13/sci16040720_af8595d47a44dfb7da915aa3847992ec_master1200.jpg","userId":"124440243","userName":"author name","seriesId":"16040720","seriesTitle":"series title","seriesContentOrder":1,"textCount":1342,"wordCount":577,"readingTime":161,"tags":["tag1","tag2"]},{"id":"28322750","title":"chapter 28322750","url":"https://i.pximg.net/c/600x600/novel-cover-master/img/2026/06/12/06/18/13/sci16040720_af8595d47a44dfb7da915aa3847992ec_master1200.jpg","userId":"124440243","userName":"author name","seriesId":"16040720","seriesTitle":"series title","seriesContentOrder":2,"textCount":3065},{"id":"28322797","title":"chapter 28322797","url":"https://i.pximg.net/c/600x600/novel-cover-master/img/2026/06/12/06/18/13/sci16040720_af8595d47a44dfb7da915aa3847992ec_master1200.jpg","userId":"124440243","userName":"author name","seriesId":"16040720","seriesTitle":"series title","seriesContentOrder":3,"textCount":3334},{"id":"28341282","title":"chapter 28341282","url":"https://i.pximg.net/c/600x600/novel-cover-master/img/2026/06/12/06/18/13/sci16040720_af8595d47a44dfb7da915aa3847992ec_master1200.jpg","userId":"124440243","userName":"author name","seriesId":"16040720","seriesTitle":"series title","seriesContentOrder":4,"textCount":3793}],"illust":[],"novelSeries":[],"novelDraft":[],"collection":[]},"illustSeries":[],"requests":[],"users":[]}}`

// 游标越过末尾的空响应:章节列表为空即表示翻页结束。
const fetchSeriesContentEmptyResponse = `{"error":false,"message":"","body":{"tagTranslation":[],"thumbnails":{"illust":[],"novel":[],"novelSeries":[],"novelDraft":[],"collection":[]},"illustSeries":[],"requests":[],"users":[],"page":{"seriesContents":[]}}}`

// fetchSeriesContentPayloadFromMock 起一个只回固定响应体的 mock 端点,
// 返回解析结果与请求到的查询串。
func fetchSeriesContentPayloadFromMock(t *testing.T, body string, opts ...FetchSeriesContentOption) (FetchSeriesContentPayload, string) {
	t.Helper()
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	p, err := FetchSeriesContent(ctx, "16040720", opts...)
	require.NoError(t, err)
	return p, gotQuery
}

func TestFetchSeriesContentChapters(t *testing.T) {
	p, _ := fetchSeriesContentPayloadFromMock(t, fetchSeriesContentResponse)
	assert.Equal(t, "16040720", p.ID())
	assert.NotEmpty(t, p.Raw())

	chapters := slices.Collect(p.Chapters())
	require.Len(t, chapters, 3)

	assert.Equal(t, "28322726", chapters[0].ID())
	assert.Equal(t, int64(1), chapters[0].Order())
	assert.Equal(t, "content 28322726", chapters[0].Title())
	// 缩略图按小说 id 关联,不依赖数组下标对齐。
	assert.Equal(t, "https://i.pximg.net/c/600x600/novel-cover-master/img/2026/06/12/06/18/13/sci16040720_af8595d47a44dfb7da915aa3847992ec_master1200.jpg", chapters[0].URL())
	assert.Equal(t, "124440243", chapters[0].AuthorID())
	assert.Equal(t, "author name", chapters[0].AuthorName())
	assert.Equal(t, "16040720", chapters[0].SeriesID())
	assert.Equal(t, "series title", chapters[0].SeriesTitle())
	assert.Equal(t, int64(1342), chapters[0].TextCount())
	assert.Equal(t, int64(577), chapters[0].WordCount())
	assert.Equal(t, int64(161), chapters[0].ReadingTime())
	assert.Equal(t, []string{"tag1", "tag2"}, slices.Collect(chapters[0].Tags()))

	assert.Equal(t, "28322797", chapters[2].ID())
	assert.Equal(t, int64(3), chapters[2].Order())
	assert.Equal(t, "content 28322797", chapters[2].Title())

	// 下一页游标为本页最大的章节次序。
	assert.Equal(t, int64(3), p.NextLastOrder())
}

func TestFetchSeriesContentRequestParams(t *testing.T) {
	// 默认升序、last_order 从 0 开始、limit 30。
	_, q := fetchSeriesContentPayloadFromMock(t, fetchSeriesContentResponse)
	assert.Equal(t, "last_order=0&limit=30&order_by=asc", q)

	_, q = fetchSeriesContentPayloadFromMock(t, fetchSeriesContentResponse,
		FetchSeriesContentWithLimit(3), FetchSeriesContentWithLastOrder(3))
	assert.Equal(t, "last_order=3&limit=3&order_by=asc", q)

	// 非法的 limit / last_order 归一为安全默认值。
	_, q = fetchSeriesContentPayloadFromMock(t, fetchSeriesContentResponse,
		FetchSeriesContentWithLimit(0), FetchSeriesContentWithLastOrder(-1))
	assert.Equal(t, "last_order=0&limit=30&order_by=asc", q)
}

func TestFetchSeriesContentEmptyPage(t *testing.T) {
	p, _ := fetchSeriesContentPayloadFromMock(t, fetchSeriesContentEmptyResponse)
	assert.Empty(t, slices.Collect(p.Chapters()))
	// 空页表示翻页结束,游标为零值。
	assert.Equal(t, int64(0), p.NextLastOrder())
}

func TestFetchSeriesContentRequiresID(t *testing.T) {
	c := client.New(client.WithServerURL("http://127.0.0.1:0"))
	ctx := client.With(context.Background(), c)
	_, err := FetchSeriesContent(ctx, "")
	require.Error(t, err)
}

// TestFetchSeriesContentLive 验证真实匿名接口的游标翻页
// (2026-09: novel series 16040720 共 11 章)。
// 匿名接口无需登录;需要网络可用并配合代理访问 pixiv。
func TestFetchSeriesContentLive(t *testing.T) {
	testenv.RequireLive(t)
	ctx := context.Background()

	p, err := FetchSeriesContent(ctx, "16040720", FetchSeriesContentWithLimit(3))
	require.NoError(t, err)
	chapters := slices.Collect(p.Chapters())
	require.Len(t, chapters, 3)
	assert.Equal(t, int64(1), chapters[0].Order())
	assert.Equal(t, "28322726", chapters[0].ID())
	assert.Equal(t, int64(3), chapters[2].Order())
	cursor := p.NextLastOrder()
	require.Equal(t, int64(3), cursor)

	// 用上一页游标取下一页。
	p2, err := FetchSeriesContent(ctx, "16040720", FetchSeriesContentWithLimit(3), FetchSeriesContentWithLastOrder(cursor))
	require.NoError(t, err)
	chapters2 := slices.Collect(p2.Chapters())
	require.Len(t, chapters2, 3)
	assert.Equal(t, int64(4), chapters2[0].Order())
	assert.Equal(t, "28341282", chapters2[0].ID())

	// 游标越过末尾得到空页。
	p3, err := FetchSeriesContent(ctx, "16040720", FetchSeriesContentWithLastOrder(11))
	require.NoError(t, err)
	assert.Empty(t, slices.Collect(p3.Chapters()))
	assert.Equal(t, int64(0), p3.NextLastOrder())
}

// 有章节但缩略图缺失的响应:缩略图来源的访问器应返回零值。
const fetchSeriesContentWithoutThumbnailResponse = `{"error":false,"message":"","body":{"page":{"seriesContents":[{"id":"1","userId":"2","title":"content 1","series":{"id":16040720,"contentOrder":1}}]},"thumbnails":{"novel":[]}}}`

func TestFetchSeriesContentChapterWithoutThumbnail(t *testing.T) {
	p, _ := fetchSeriesContentPayloadFromMock(t, fetchSeriesContentWithoutThumbnailResponse)
	chapters := slices.Collect(p.Chapters())
	require.Len(t, chapters, 1)
	c := chapters[0]
	// 章节自身的字段照常可读。
	assert.Equal(t, "1", c.ID())
	assert.Equal(t, int64(1), c.Order())
	assert.Equal(t, "content 1", c.Title())
	// 缩略图缺失时相关访问器返回零值而不是报错。
	assert.Equal(t, "", c.URL())
	assert.Equal(t, "", c.AuthorID())
	assert.Equal(t, "", c.AuthorName())
	assert.Equal(t, "", c.SeriesTitle())
	assert.Equal(t, int64(0), c.TextCount())
}
