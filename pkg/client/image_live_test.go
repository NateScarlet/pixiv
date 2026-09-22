package client

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这些用例断言真实 pixiv 图片主机当下的行为，失败可能源于 pixiv 改版或网络侧
// 策略变化，故加真实网络门禁：置 PIXIV_LIVE=1 才运行。
//
// 经代理运行时置 HTTPS_PROXY 即可（客户端默认传输会自行遵循代理设置）。

// liveImageURLs 是同一作品（149365161「夏」）各尺寸的实测地址。
//
// 这些地址取自该作品的详情接口，不是按命名规则猜的：原图的扩展名与
// 其它尺寸不同（实测为 .png）。
const (
	liveImageRegularURL  = "https://i.pximg.net/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg"
	liveImageOriginalURL = "https://i.pximg.net/img-original/img/2026/09/07/00/00/12/149365161_p0.png"
	liveImageMiniURL     = "https://i.pximg.net/c/48x48/img-master/img/2026/09/07/00/00/12/149365161_p0_square1200.jpg"
	// AuthorProfileImageURL() 一类访问器交给调用者的地址（实测取自排行榜响应）。
	liveImageProfileURL = "https://i.pximg.net/user-profile/img/2022/09/23/01/34/52/23368434_0daa45f98a51e102a4ef48411bffe087_50.jpg"
	// liveUgoiraZipURL 是真实动图作品（44332434）的压缩版 zip 地址，实测取自
	// 其 ugoira_meta 接口（src 字段），路径段为 img-zip-ugoira。
	liveUgoiraZipURL = "https://i.pximg.net/img-zip-ugoira/img/2014/06/27/00/20/58/44332434_ugoira600x600.zip"
)

// TestFetchImageLive 断言真实图片主机上取回成功：
// 附带 Referer 后返回 200，且响应头可供调用者读取格式与长度。
func TestFetchImageLive(t *testing.T) {
	testenv.RequireLive(t)
	c := New()

	resp, err := c.FetchImage(context.Background(), liveImageRegularURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))
	assert.NotEmpty(t, resp.Header.Get("Content-Length"))
}

// TestFetchImageLiveWithoutRefererIsRejected 断言不携带 Referer 的同一请求
// 被图片主机拒绝。这是本方法代为附加 Referer 的依据：
// 要求无法从图片 URL 推知，调用者自己拼请求只会看到 403。
func TestFetchImageLiveWithoutRefererIsRejected(t *testing.T) {
	testenv.RequireLive(t)
	c := New()
	// 清掉默认请求头里可能存在的 Referer，模拟调用者自行拼请求。
	c.SetRequestOptions(func(req *http.Request) {
		req.Header.Del("Referer")
	})

	req, err := http.NewRequest(http.MethodGet, liveImageRegularURL, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"对照组：不带 Referer 时图片主机应拒绝请求")
}

// TestFetchImageLiveOriginalBytesMatchContentLength 断言取回的字节数与
// Content-Length 一致，即响应体未被截断或转码。
//
// 原图的实际格式与其它尺寸不同（这里是 image/png），格式以响应为准。
func TestFetchImageLiveOriginalBytesMatchContentLength(t *testing.T) {
	testenv.RequireLive(t)
	c := New()

	resp, err := c.FetchImage(context.Background(), liveImageOriginalURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, resp.Header.Get("Content-Length"), strconv.Itoa(len(got)),
		"取回字节数应与 Content-Length 一致")
	assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
}

// TestFetchImageLiveMiniWorksWithoutAssumingContentLength 断言 mini 尺寸可正常
// 取回，且实现不假定 Content-Length 总存在。
//
// 实测该头并非所有路径都返回（直连与该主机经代理时观测到的结果不同），
// 因此用例只要求取回成功、字节可读，并断言字节数与头一致（头存在时）——
// 这样它在两种观测下都成立，不会把一个会随网络路径变化的现象写死成断言。
func TestFetchImageLiveMiniWorksWithoutAssumingContentLength(t *testing.T) {
	testenv.RequireLive(t)
	c := New()

	resp, err := c.FetchImage(context.Background(), liveImageMiniURL)
	require.NoError(t, err, "缺少 Content-Length 不应影响取回")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.NotEmpty(t, got, "响应体应可读")
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		assert.Equal(t, cl, strconv.Itoa(len(got)), "头存在时字节数应与之一致")
	}
}

// TestFetchImageLiveAuthorProfileURL 断言库自身交给调用者的头像地址也可直接
// 取回（user story 18：从 Payload 取得的 URL 可直接传入本方法）。
//
// 这类地址不具备画作那样的各尺寸结构，因此判读必须按路径段而非按能否重建尺寸。
func TestFetchImageLiveAuthorProfileURL(t *testing.T) {
	testenv.RequireLive(t)
	c := New()

	resp, err := c.FetchImage(context.Background(), liveImageProfileURL)
	require.NoError(t, err, "库自身提供的头像地址应可取得")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.NotEmpty(t, got)
}

// TestFetchImageLiveUgoiraZip 断言动图 zip 与图片一样可经本方法取回：
// 路径段 img-zip-ugoira 被识别，自动携带 Referer 后源站返回 200 与 zip 内容。
func TestFetchImageLiveUgoiraZip(t *testing.T) {
	testenv.RequireLive(t)
	c := New()

	resp, err := c.FetchImage(context.Background(), liveUgoiraZipURL)
	require.NoError(t, err, "动图 zip 应可取回")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/zip", resp.Header.Get("Content-Type"))

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		assert.Equal(t, cl, strconv.Itoa(len(got)), "头存在时字节数应与之一致")
	}
}
