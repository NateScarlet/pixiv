package artwork

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ugoiraMetaResponse 是动图元数据响应样本(mock)，字段名与真实 /ajax/illust/{id}/ugoira_meta
// 一致：src / originalSrc 为 zip 地址，frames 中 delay 单位毫秒。
const ugoiraMetaResponse = `{"error":false,"body":{"src":"https://i.pximg.net/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira600x600.zip","originalSrc":"https://i.pximg.net/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira1920x1080.zip","mime_type":"image/gif","frames":[{"file":"000000.jpg","delay":30},{"file":"000001.jpg","delay":0},{"file":"000002.jpg","delay":100}]}}`

// fetchUgoiraMetaFromMock 启动本地端点返回给定响应，并用该端点装配客户端，
// 使被测方法不依赖真实网络。
func fetchUgoiraMetaFromMock(t *testing.T, body string) (UgoiraMeta, error) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	c := client.New(client.WithServerURL(server.URL))
	ctx := client.With(context.Background(), c)

	return FetchUgoiraMeta(ctx, "149365161")
}

// TestFetchUgoiraMetaParsesResponse 断言完整解析 zip 地址、格式与帧时序：
// delay 毫秒转成 time.Duration，帧按数组顺序即播放顺序。
func TestFetchUgoiraMetaParsesResponse(t *testing.T) {
	m, err := fetchUgoiraMetaFromMock(t, ugoiraMetaResponse)
	require.NoError(t, err)

	assert.Equal(t, "149365161", m.ID())
	assert.Equal(t, "https://i.pximg.net/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira600x600.zip", m.ZipURL())
	assert.Equal(t, "https://i.pximg.net/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira1920x1080.zip", m.OriginalZipURL())
	assert.Equal(t, "image/gif", m.MimeType())

	frames := slices.Collect(m.Frames())
	require.Len(t, frames, 3)
	assert.Equal(t, []UgoiraFrame{
		{File: "000000.jpg", Delay: 30 * time.Millisecond},
		{File: "000001.jpg", Delay: 0},
		{File: "000002.jpg", Delay: 100 * time.Millisecond},
	}, frames)

	assert.NotEmpty(t, m.Raw())
	assert.NotNil(t, m.Response())
}

// TestFetchUgoiraMetaReportsAPIError 断言服务端 error 信封（如对非动图作品）
// 以错误形式给出，调用者可据此快速失败。
func TestFetchUgoiraMetaReportsAPIError(t *testing.T) {
	_, err := fetchUgoiraMetaFromMock(t, `{"error":true,"message":"動画情報は存在しません"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "動画情報は存在しません")
}

// TestFetchUgoiraMetaRequiresID 断言空 id 在发出请求前即失败。
func TestFetchUgoiraMetaRequiresID(t *testing.T) {
	_, err := FetchUgoiraMeta(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

// TestFetchUgoiraMetaRejectsInvalidJSON 断言非法 JSON 响应以错误给出，
// 而不是把不可解析的内容当元数据返回。
func TestFetchUgoiraMetaRejectsInvalidJSON(t *testing.T) {
	_, err := fetchUgoiraMetaFromMock(t, `not json`)
	require.Error(t, err)
}
