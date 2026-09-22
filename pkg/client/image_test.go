package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 取回图片的用例只断言外部行为：发出的请求具备哪些可观测特征（是否带
// Referer、目标地址），以及方法对各类响应返回什么。它们全部使用
// httptest 本地端点，不依赖真实网络，也不断言库持有的主机清单内容。

// testImageBytes 是一段带有非 ASCII 与二进制字节的内容，用于断言响应体
// 未被转码：内容经由 httptest 往返后应逐字节一致。
var testImageBytes = append(
	[]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, // JPEG 头，含 NUL 字节
	[]byte("图像字节校验\x00\xff")...,
)

// testImagePath 是本地端点上的图片路径，与真实 pixiv 图片地址同构，
// 因此被测方法会把它当作图片地址接受。
const testImagePath = "/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg"

// newImageServer 启动一个本地图片端点，并返回一个把请求转给该端点、
// 同时记录请求的客户端：断言请求的可观测特征与断言响应内容因此可以同源。
//
// 传输使用进程默认传输的克隆（httptest 的地址是 IP 字面量，不需要库自带的
// 连接能力），因此用例不依赖真实网络，也不受运行机器的解析环境影响。
func newImageServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client, *spyTransport) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	spy := &spyTransport{}
	// 转发链：记录请求 → 交给本地端点。
	spy.next = defaultBaseTransport()
	return server, New(WithTransport(spy)), spy
}

// serveImageBytes 以给定的 Content-Type 返回测试图片字节。
func serveImageBytes(contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", fmt.Sprint(len(testImageBytes)))
		w.WriteHeader(http.StatusOK)
		w.Write(testImageBytes)
	}
}

// TestFetchImageReturnsReadableResponse 断言传入了图片 URL 就能拿到可读的
// 响应：状态码、Content-Type、Content-Length 与内容都来自源站。
func TestFetchImageReturnsReadableResponse(t *testing.T) {
	server, c, spy := newImageServer(t, serveImageBytes("image/jpeg"))

	resp, err := c.FetchImage(context.Background(), server.URL+testImagePath)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))
	assert.Equal(t, fmt.Sprint(len(testImageBytes)), resp.Header.Get("Content-Length"))
	assert.Equal(t, server.URL+testImagePath, spy.last().URL.String(), "请求应发往传入的图片地址")
}

// TestFetchImageDoesNotTranscodeBody 断言返回的字节与源站返回的一致，
// 未被转码或重新压缩，因此调用者可以据此校验哈希。
func TestFetchImageDoesNotTranscodeBody(t *testing.T) {
	server, c, _ := newImageServer(t, serveImageBytes("image/png"))

	resp, err := c.FetchImage(context.Background(), server.URL+testImagePath)
	require.NoError(t, err)
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(testImageBytes, got), "响应体字节应与源站返回的一致，实际 %q", got)
}

// TestFetchImageStreamsWithoutBuffering 断言响应体可流式读取：
// 未读完整个响应也能从流中依次取到前缀字节。
func TestFetchImageStreamsWithoutBuffering(t *testing.T) {
	server, c, _ := newImageServer(t, serveImageBytes("image/jpeg"))

	resp, err := c.FetchImage(context.Background(), server.URL+testImagePath)
	require.NoError(t, err)
	defer resp.Body.Close()

	prefix := make([]byte, 4)
	_, err = io.ReadFull(resp.Body, prefix)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(testImageBytes[:4], prefix), "应能逐段从响应流读取，实际 %q", prefix)
}

// TestFetchImageSetsReferer 断言请求自动附带 pixiv 主站 Referer：
// 实测不附带时图片主机返回 403，而调用者无从得知这一要求。
func TestFetchImageSetsReferer(t *testing.T) {
	server, c, spy := newImageServer(t, serveImageBytes("image/jpeg"))

	resp, err := c.FetchImage(context.Background(), server.URL+testImagePath)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "https://www.pixiv.net/", spy.last().Header.Get("Referer"))
}

// TestFetchImageKeepsCallerReferer 断言调用者已设置的 Referer 不被覆盖。
func TestFetchImageKeepsCallerReferer(t *testing.T) {
	server, c, spy := newImageServer(t, serveImageBytes("image/jpeg"))
	c.SetRequestOptions(func(req *http.Request) {
		req.Header.Set("Referer", "https://example.com/gallery")
	})

	resp, err := c.FetchImage(context.Background(), server.URL+testImagePath)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "https://example.com/gallery", spy.last().Header.Get("Referer"))
}

// TestFetchImageRejectsNonImageURL 断言传入非图片地址时快速失败，
// 而不是发出一个注定无用的请求；错误可用 errors.Is 辨认。
func TestFetchImageRejectsNonImageURL(t *testing.T) {
	spy := &spyTransport{}
	c := New(WithTransport(spy))

	for _, tt := range []struct {
		name     string
		imageURL string
	}{
		{"空地址", ""},
		{"画作页面地址", "https://www.pixiv.net/artworks/149365161"},
		{"API 地址", "https://www.pixiv.net/ajax/illust/149365161"},
		{"图片主机上的非图片路径", "https://i.pximg.net/common/images/limit_unviewable_s.png"},
		{"缺少主机名", "/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg"},
		{"非 http 协议", "ftp://i.pximg.net/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg"},
		{"无法解析", "://invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := c.FetchImage(context.Background(), tt.imageURL)
			require.Error(t, err)
			assert.Nil(t, resp, "失败时不应返回响应")
			assert.ErrorIs(t, err, ErrImageURLNotRecognized)
		})
	}
	assert.Equal(t, 0, spy.count(), "非图片地址不应发出请求")
}

// TestFetchImageAcceptsEverySizeURL 断言各尺寸与各类图片路径都可取回：
// 调用者不必区分尺寸，也不必知道有哪些路径段。
//
// 覆盖库自身会交给调用者的各类图片地址，其中 user-profile / background
// 来自 AuthorProfileImageURL() 等访问器（user story 18）。
func TestFetchImageAcceptsEverySizeURL(t *testing.T) {
	server, c, _ := newImageServer(t, serveImageBytes("image/jpeg"))

	for _, tt := range []struct {
		name string
		path string
	}{
		{"mini", "/c/48x48/img-master/img/2026/09/07/00/00/12/149365161_p0_square1200.jpg"},
		{"thumb", "/c/250x250_80_a2/img-master/img/2026/09/07/00/00/12/149365161_p0_square1200.jpg"},
		{"small", "/c/540x540_70/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg"},
		{"regular", testImagePath},
		{"original", "/img-original/img/2026/09/07/00/00/12/149365161_p0.png"},
		{"自定义裁剪缩略图", "/c/250x250_80_a2/custom-thumb/img/2026/09/07/00/00/12/149365161_p0_custom1200.jpg"},
		{"小说封面", "/novel-cover-original/img/2026/09/07/00/00/12/149365161_p0.png"},
		{"带缩放的小说封面", "/c/600x600/novel-cover-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg"},
		{"作者头像", "/user-profile/img/2026/09/07/00/00/12/23368434_0daa45f98a51e102a4ef48411bffe087_50.jpg"},
		{"用户背景图", "/background/img/2026/09/07/00/00/12/23368434_abc.jpg"},
		{"动图压缩版 zip", "/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira600x600.zip"},
		{"动图原图 zip", "/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira1920x1080.zip"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := c.FetchImage(context.Background(), server.URL+tt.path)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

// TestFetchImageReportsRejection 断言请求被拒绝时给出可行动的措辞，
// 而不是只透出裸状态码。
func TestFetchImageReportsRejection(t *testing.T) {
	server, c, _ := newImageServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "<html>forbidden</html>")
	})

	resp, err := c.FetchImage(context.Background(), server.URL+testImagePath)
	require.Error(t, err)
	assert.Nil(t, resp, "失败时不应返回响应")
	assert.ErrorIs(t, err, ErrImageRejected)
	assert.Contains(t, err.Error(), "403", "应让调用者看到状态码")
}

// TestFetchImageClosesBodyOnFailure 断言失败路径下响应体被关闭，不泄漏连接。
func TestFetchImageClosesBodyOnFailure(t *testing.T) {
	body := &trackingReadCloser{Reader: strings.NewReader("forbidden")}
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       body,
			Request:    req,
		}, nil
	})
	c := New(WithTransport(rt))

	_, err := c.FetchImage(context.Background(), "https://i.pximg.net/img-master/x.jpg")
	require.Error(t, err)
	assert.True(t, body.closed, "失败路径下响应体应被关闭")
}

// TestFetchImageReportsTransportFailure 断言主机不可达或解析失败时，
// 错误保留底层原因并说明主机不可达。
func TestFetchImageReportsTransportFailure(t *testing.T) {
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("stub: 解析主机失败")
	})
	c := New(WithTransport(rt))

	resp, err := c.FetchImage(context.Background(), "https://i.pximg.net/img-master/x.jpg")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrImageHostUnreachable)
	assert.Contains(t, err.Error(), "i.pximg.net")
	assert.Contains(t, err.Error(), "stub: 解析主机失败", "应保留底层原因以便调用者区分 DNS 污染与网络封锁")
}

// TestFetchImageClosesBodyErrorIsReported 断言关闭响应体失败不会让错误处理
// 静默吞掉该信息：它并入返回的错误，调用者可见。
func TestFetchImageClosesBodyErrorIsReported(t *testing.T) {
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       &trackingReadCloser{Reader: strings.NewReader("x"), closeErr: errors.New("stub: 关闭失败")},
			Request:    req,
		}, nil
	})
	c := New(WithTransport(rt))

	_, err := c.FetchImage(context.Background(), "https://i.pximg.net/img-master/x.jpg")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrImageRejected, "分类不因关闭失败而改变")
	assert.Contains(t, err.Error(), "stub: 关闭失败", "关闭失败应可见")
}

// TestFetchImageHonorsContextCancellation 断言 context 取消会中止取回，
// 调用者可用 errors.Is 辨认取消。
func TestFetchImageHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New(WithTransport(&AutoTransport{Base: defaultBaseTransport()}))

	resp, err := c.FetchImage(ctx, server.URL+testImagePath)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, context.Canceled), "应可用 errors.Is 辨认取消，实际 %v", err)
}

// TestFetchImageUsesClientTransport 断言取回走的是客户端自己的传输，
// 因此主机由传输层分派，方法本身不重复这套判断。
func TestFetchImageUsesClientTransport(t *testing.T) {
	var gotHost string
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		gotHost = req.URL.Host
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("x")),
			Request:    req,
		}, nil
	})
	c := New(WithTransport(rt))

	resp, err := c.FetchImage(context.Background(), "https://i.pximg.net/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "i.pximg.net", gotHost)
}

// trackingReadCloser 记录响应体是否被关闭，并可模拟关闭失败。
type trackingReadCloser struct {
	io.Reader
	closed   bool
	closeErr error
}

func (c *trackingReadCloser) Close() error {
	c.closed = true
	return c.closeErr
}

var _ io.ReadCloser = (*trackingReadCloser)(nil)
