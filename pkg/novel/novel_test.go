package novel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/NateScarlet/snapshot/pkg/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapshotNovel(t *testing.T, novel Novel, opts ...snapshot.Option) {
	snapshot.MatchJSON(
		t,
		novel,
		append(
			[]snapshot.Option{
				snapshot.OptionCleanRegex(
					snapshot.CleanAs(`"*count*"`),
					`(?m)^\s*"(?:View|Like|Comment|Bookmark)Count": (\d+),?$`,
				),
			},
			opts...,
		)...,
	)
}

func TestFetchNovel(t *testing.T) {
	if os.Getenv("PIXIV_PHPSESSID") == "" {
		t.Skip("need login")
	}
	var ctx = context.Background()
	i := Novel{ID: "11983096"}
	err := i.Fetch(ctx)
	require.NoError(t, err)
	t.Log(i)
	assert.Equal(t, "11983096", i.ID)
	assert.Equal(t, "転生したら獪岳になってんだが俺はこいつのことをよく知らない・玖", i.Title)
	assert.GreaterOrEqual(t, len(i.Tags), 6)
	created, err := time.Parse(time.RFC3339, "2019-11-21T16:49:02+00:00")
	require.NoError(t, err)
	assert.Equal(t, created, i.Created)
	assert.Equal(t, "41540476", i.Author.ID)
	assert.NotEmpty(t, i.Author.Name)
	assert.NotEmpty(t, i.Content)
	assert.Equal(t, int64(6), i.PageCount)
	assert.GreaterOrEqual(t, i.CommentCount, int64(72))
	assert.GreaterOrEqual(t, i.LikeCount, int64(3178))
	assert.GreaterOrEqual(t, i.ViewCount, int64(21955))
	assert.GreaterOrEqual(t, i.BookmarkCount, int64(3690))
	u := i.URL(ctx)
	assert.Equal(t, "https://www.pixiv.net/novel/show.php?id=11983096", u.String())
}

func TestFetchNovelWithEmbeddedImages(t *testing.T) {
	if os.Getenv("PIXIV_PHPSESSID") == "" {
		t.Skip("need login")
	}
	var ctx = context.Background()
	i := Novel{ID: "14443124"}
	err := i.Fetch(ctx)
	require.NoError(t, err)
	snapshotNovel(t, i)
}

func TestFetchNovelNullEmbeddedImages(t *testing.T) {
	// 模拟 /ajax/novel/{id} 返回 textEmbeddedImages 为 null 的响应。
	// 该场景对应无嵌入图的小说（如 29047862）。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"body":{"title":"","description":"","coverUrl":"","content":"","createDate":"","uploadDate":"","userId":"","userName":"","pageCount":0,"commentCount":0,"likeCount":0,"viewCount":0,"bookmarkCount":0,"tags":{"tags":[]},"textEmbeddedImages":null,"aiType":0}}`)
	}))
	defer server.Close()

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	i := Novel{ID: "29047862"}
	err := i.Fetch(ctx)
	require.NoError(t, err)
	assert.Nil(t, i.EmbeddedImages)
}

// seriesNavData 含系列信息的小说详情响应(mock),基于 issue #84 的样本(novel 28612019)。
const fetchNovelWithSeriesResponse = `{"body":{"title":"もし、ボーイズバーで働くとして","seriesNavData":{"seriesType":"novel","seriesId":16040720,"title":"もし、ボーイズバーで働くとして","order":11,"prev":{"title":"第９話","order":10,"id":"28556784","available":true},"next":null},"aiType":1}}`

func fetchNovelFromMock(t *testing.T, body string) Novel {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	i := Novel{ID: "28612019"}
	err := i.Fetch(ctx)
	require.NoError(t, err)
	return i
}

func TestFetchNovelFillsSeriesFromSeriesNavData(t *testing.T) {
	i := fetchNovelFromMock(t, fetchNovelWithSeriesResponse)
	assert.Equal(t, "16040720", i.Series.ID)
	assert.Equal(t, "もし、ボーイズバーで働くとして", i.Series.Title)
}

func TestFetchNovelWithoutSeriesNavDataKeepsZeroSeries(t *testing.T) {
	i := fetchNovelFromMock(t, `{"body":{"title":"no series"}}`)
	assert.Equal(t, Series{}, i.Series)
}

// TestFetchNovelSeriesLive 验证真实匿名接口解析 seriesNavData(issue #84 样本: 28612019)。
// 匿名接口无需登录;需要网络可用并配合代理访问 pixiv。
func TestFetchNovelSeriesLive(t *testing.T) {
	testenv.RequireLive(t)
	i := Novel{ID: "28612019"}
	err := i.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "16040720", i.Series.ID)
	assert.Equal(t, "もし、ボーイズバーで働くとして", i.Series.Title)
}
