package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/url"
	"strconv"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/tidwall/gjson"
)

// FetchSeries fetches an artwork series page,
// returns an immutable payload: known fields are exposed as methods,
// unknown fields are available from Raw.
// Fetch 获取作品系列页面,返回不可变记录:已知字段通过方法访问,未知字段通过 Raw 获取。
func FetchSeries(ctx context.Context, id string, options ...FetchSeriesOption) (_ FetchSeriesPayload, err error) {
	if id == "" {
		err = errors.New("pixiv: artwork.FetchSeries: id is required")
		return
	}
	var opts = newFetchSeriesOptions(options...)
	// 服务端要求 p 参数,缺失时返回 400。
	page := opts.page
	if page < 1 {
		page = 1
	}
	q := make(url.Values)
	q.Set("p", strconv.Itoa(page))
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/series/"+id, &q).String())
	if err != nil {
		return
	}
	body, err := client.ParseAPIResponseV2(resp)
	if err != nil {
		return
	}
	return FetchSeriesPayload{id, body, resp}, nil
}

func newFetchSeriesOptions(options ...FetchSeriesOption) *FetchSeriesOptions {
	var opts = new(FetchSeriesOptions)
	for _, i := range options {
		i(opts)
	}
	return opts
}

type FetchSeriesOptions struct {
	page int
}

type FetchSeriesOption func(*FetchSeriesOptions)

// FetchSeriesWithPage sets the series page to fetch (1-based, default 1).
// FetchSeriesWithPage 设置要获取的系列页码(从 1 开始,默认第 1 页)。
func FetchSeriesWithPage(page int) FetchSeriesOption {
	return func(opts *FetchSeriesOptions) {
		opts.page = page
	}
}

// FetchSeriesPayload is the immutable record of an artwork series page response.
// FetchSeriesPayload 是作品系列页面响应的不可变记录。
type FetchSeriesPayload struct {
	id   string
	raw  json.RawMessage
	resp *http.Response
}

func (p FetchSeriesPayload) get(path string) gjson.Result {
	return gjson.GetBytes(p.raw, path)
}

// ID returns the requested series id.
// ID 返回请求的系列 ID。
func (p FetchSeriesPayload) ID() string {
	return p.id
}

// Raw returns the raw response body.
// Raw 返回响应体原始 JSON。
func (p FetchSeriesPayload) Raw() json.RawMessage {
	return p.raw
}

// Response returns the HTTP response.
// Response 返回 HTTP 响应。
func (p FetchSeriesPayload) Response() *http.Response {
	return p.resp
}

// metadata returns the requested series entry of illustSeries,
// a zero result when absent.
// metadata 返回 illustSeries 中属于本系列的条目;缺失时为零值。
func (p FetchSeriesPayload) metadata() gjson.Result {
	var ret gjson.Result
	p.get("illustSeries").ForEach(func(_, value gjson.Result) bool {
		if value.Get("id").String() == p.id {
			ret = value
			return false
		}
		return true
	})
	return ret
}

// Title returns the series title.
// Title 返回系列标题。
func (p FetchSeriesPayload) Title() string {
	return p.metadata().Get("title").String()
}

// Total returns the total number of chapters in the series.
// Total 返回系列的章节总数。
func (p FetchSeriesPayload) Total() int64 {
	return p.metadata().Get("total").Int()
}

// FirstIllustID returns the first chapter's artwork id.
// FirstIllustID 返回首章的作品 ID。
func (p FetchSeriesPayload) FirstIllustID() string {
	return p.metadata().Get("firstIllustId").String()
}

// LatestIllustID returns the latest chapter's artwork id.
// LatestIllustID 返回最新一章的作品 ID。
func (p FetchSeriesPayload) LatestIllustID() string {
	return p.metadata().Get("latestIllustId").String()
}

// CoverURL returns the series cover URL.
// CoverURL 返回系列封面 URL。
func (p FetchSeriesPayload) CoverURL() string {
	return p.metadata().Get("url").String()
}

// Chapters returns an iterator over the chapter list of the current page.
// Chapters 返回当前页章节列表的迭代器。
func (p FetchSeriesPayload) Chapters() iter.Seq[ChapterInFetchSeriesPayload] {
	return func(yield func(ChapterInFetchSeriesPayload) bool) {
		p.get("page.series").ForEach(func(_, value gjson.Result) bool {
			return yield(ChapterInFetchSeriesPayload{payload: p, raw: json.RawMessage(value.Raw)})
		})
	}
}

// ChapterInFetchSeriesPayload is a single chapter of an artwork series page.
// ChapterInFetchSeriesPayload 表示作品系列页面中的一个章节。
type ChapterInFetchSeriesPayload struct {
	payload FetchSeriesPayload
	raw     json.RawMessage
}

func (c ChapterInFetchSeriesPayload) get(path string) gjson.Result {
	return gjson.GetBytes(c.raw, path)
}

// Raw returns the raw chapter JSON.
// Raw 返回章节的原始 JSON。
func (c ChapterInFetchSeriesPayload) Raw() json.RawMessage {
	return c.raw
}

// ID returns the chapter's artwork id.
// ID 返回章节的作品 ID。
func (c ChapterInFetchSeriesPayload) ID() string {
	return c.get("workId").String()
}

// Order returns the chapter's position in the series (1-based).
// Order 返回章节在系列中的次序(从 1 开始)。
func (c ChapterInFetchSeriesPayload) Order() int64 {
	return c.get("order").Int()
}

// thumbnail returns the thumbnail entry matching this chapter's artwork id,
// a zero result when absent.
// thumbnail 返回与本章节作品 ID 匹配的缩略图条目;缺失时为零值。
func (c ChapterInFetchSeriesPayload) thumbnail() gjson.Result {
	var ret gjson.Result
	c.payload.get("thumbnails.illust").ForEach(func(_, value gjson.Result) bool {
		if value.Get("id").String() == c.ID() {
			ret = value
			return false
		}
		return true
	})
	return ret
}

// Title returns the chapter title from the thumbnail.
// Title 返回章节标题(来自缩略图)。
func (c ChapterInFetchSeriesPayload) Title() string {
	return c.thumbnail().Get("title").String()
}

// URL returns the chapter thumbnail URL.
// URL 返回章节缩略图 URL。
func (c ChapterInFetchSeriesPayload) URL() string {
	return c.thumbnail().Get("url").String()
}

// AuthorID returns the author's unique identifier.
// AuthorID 返回作者唯一标识符。
func (c ChapterInFetchSeriesPayload) AuthorID() string {
	return c.thumbnail().Get("userId").String()
}

// AuthorName returns the author's display name.
// AuthorName 返回作者显示名称。
func (c ChapterInFetchSeriesPayload) AuthorName() string {
	return c.thumbnail().Get("userName").String()
}
