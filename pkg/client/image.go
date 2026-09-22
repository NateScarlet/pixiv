package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/NateScarlet/pixiv/pkg/image"
)

// pixiv: client: FetchImage: 的分类错误，供调用者用 [errors.Is] 分支处理，
// 而不必匹配错误文本。三类分别对应「传错了参数」「服务端拒绝了这次请求」
// 「根本没连上主机（含解析失败）」。
var (
	// ErrImageURLNotRecognized 表示入参不是可识别的 pixiv 图片或动图 zip 地址。
	ErrImageURLNotRecognized = errors.New("pixiv: client: 不是可识别的 pixiv 图片或动图 zip 地址")
	// ErrImageRejected 表示服务端以非成功状态码拒绝了这次请求。
	ErrImageRejected = errors.New("pixiv: client: 图片请求被拒绝")
	// ErrImageHostUnreachable 表示主机不可达或无法解析。
	ErrImageHostUnreachable = errors.New("pixiv: client: 图片主机不可达")
)

// imageFetchReferer 是取回图片时必须携带的 Referer。
//
// 实测图片主机不带 Referer 时返回 403、带 pixiv 主站地址时返回 200。
// 该要求无法从图片 URL 推知，因此由本方法代为附加；调用者已设置时不覆盖。
const imageFetchReferer = "https://www.pixiv.net/"

// FetchImage 取回图片或动图 zip 的内容，返回可读的响应，由调用者自行消费——
// 写文件、解码、计算哈希或流式处理皆可。
//
// 它处理调用者无从得知的部分：
//
//   - 请求自动携带图片主机要求的 Referer（调用者已设置时不覆盖）；
//   - 失败时给出可行动的说明，并可用 [errors.Is] 分辨类别：
//     地址不可识别（[ErrImageURLNotRecognized]）、请求被拒绝（[ErrImageRejected]）、
//     主机不可达或解析失败（[ErrImageHostUnreachable]）。底层原因保留在错误里。
//
// 动图 zip 地址（img-zip-ugoira 路径段，来自 [FetchUgoiraMeta] 之类元数据接口）
// 与图片同主机、同样要求 Referer，因此一并由此方法取回。
//
// 主机与其接入方式由客户端的传输决定（默认传输会按主机选用合适的方式），
// 本方法不做这套判断，也不复制一份主机清单。
//
// 响应体未被转码或重新压缩，字节与源站返回的一致，因此可据其校验哈希；
// 内容格式从响应的 Content-Type 读取，不要按 URL 扩展名推断（原图尤甚）。
// 响应体由调用者负责关闭；失败路径下响应体已由本方法关闭，并从返回值中移除，
// 因此无需（也无法）再关闭一次。
//
// ctx 被取消时取回中止，错误可用 errors.Is(err, context.Canceled) 辨认。
func (c *Client) FetchImage(ctx context.Context, imageURL string) (*http.Response, error) {
	if err := checkImageURL(imageURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		// 地址已在 checkImageURL 校验过，此处失败只可能源于方法或控制字符等
		// 非地址问题，附上 ErrImageURLNotRecognized 会是错误的分类，故原样返回。
		return nil, err
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", imageFetchReferer)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: 无法访问主机 %s: %w", ErrImageHostUnreachable, req.URL.Host, err)
	}
	if resp.StatusCode != http.StatusOK {
		// 只有 200 是成功：206 之类的部分内容会给出被截断的字节，
		// 与「字节与源站返回一致」相冲突，调用者不应把它当作取回成功。
		// 非 200 的响应体对调用者无用，关闭它以复用连接；失败时只返回错误。
		// 关闭失败不改变结论（已决定报错），但它是调用者可见的诊断信息。
		var closeErr = resp.Body.Close()
		return nil, errors.Join(fmt.Errorf("%w: 请求 %s 返回 %s",
			ErrImageRejected, imageURL, resp.Status), closeErr)
	}
	return resp, nil
}

// checkImageURL 在发出请求前拒绝不是 pixiv CDN 资源地址（图片或动图 zip）的
// 入参，使调用者尽早发现传错了参数。
//
// 判断「是不是可取资源」的路径布局知识由 [image.IsImageURL] 持有，本包不重复
// 一份，以免两处清单各自漂移。这里只把布尔结论转成调用者可读的错误。
func checkImageURL(imageURL string) error {
	if image.IsImageURL(imageURL) {
		return nil
	}
	return fmt.Errorf("%w: %s 的路径不是 pixiv 图片或动图 zip 地址（应为 %s 之一）",
		ErrImageURLNotRecognized, imageURL, strings.Join(image.PathSegments(), " / "))
}
