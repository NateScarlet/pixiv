package novel

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

// defaultFetchSeriesContentLimit is the chapter count per request.
// defaultFetchSeriesContentLimit 是每次请求返回的章节数。
const defaultFetchSeriesContentLimit = 30

// FetchSeriesContent fetches one page of a novel series' chapter list,
// in ascending chapter order, returns an immutable payload:
// known fields are exposed as methods, unknown fields are available from Raw.
// Fetch 获取小说系列的一页章节列表(按章节次序升序),返回不可变记录:
// 已知字段通过方法访问,未知字段通过 Raw 获取。
func FetchSeriesContent(ctx context.Context, id string, options ...FetchSeriesContentOption) (_ FetchSeriesContentPayload, err error) {
	if id == "" {
		err = errors.New("pixiv: novel.FetchSeriesContent: id is required")
		return
	}
	var opts = newFetchSeriesContentOptions(options...)
	// 非法的调用参数归一为安全默认值,而不是把零值/负值传给服务端。
	if opts.limit < 1 {
		opts.limit = defaultFetchSeriesContentLimit
	}
	if opts.lastOrder < 0 {
		opts.lastOrder = 0
	}
	q := make(url.Values)
	q.Set("order_by", "asc")
	q.Set("last_order", strconv.FormatInt(opts.lastOrder, 10))
	q.Set("limit", strconv.Itoa(opts.limit))
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/novel/series_content/"+id, &q).String())
	if err != nil {
		return
	}
	body, err := client.ParseAPIResponseV2(resp)
	if err != nil {
		return
	}
	return FetchSeriesContentPayload{id, body, resp}, nil
}

func newFetchSeriesContentOptions(options ...FetchSeriesContentOption) *FetchSeriesContentOptions {
	var opts = &FetchSeriesContentOptions{limit: defaultFetchSeriesContentLimit}
	for _, i := range options {
		i(opts)
	}
	return opts
}

type FetchSeriesContentOptions struct {
	limit     int
	lastOrder int64
}

type FetchSeriesContentOption func(*FetchSeriesContentOptions)

// FetchSeriesContentWithLimit sets the chapter count per request (default 30).
// FetchSeriesContentWithLimit 设置每次请求返回的章节数(默认 30)。
func FetchSeriesContentWithLimit(limit int) FetchSeriesContentOption {
	return func(opts *FetchSeriesContentOptions) {
		opts.limit = limit
	}
}

// FetchSeriesContentWithLastOrder sets the cursor: chapters with an order
// greater than this are returned, 0 starts from the first chapter.
// FetchSeriesContentWithLastOrder 设置游标:返回次序大于该值的章节,0 表示从首章开始。
func FetchSeriesContentWithLastOrder(lastOrder int64) FetchSeriesContentOption {
	return func(opts *FetchSeriesContentOptions) {
		opts.lastOrder = lastOrder
	}
}

// FetchSeriesContentPayload is the immutable record of a novel series
// chapter list response.
// FetchSeriesContentPayload 是小说系列章节列表响应的不可变记录。
type FetchSeriesContentPayload struct {
	id   string
	raw  json.RawMessage
	resp *http.Response
}

func (p FetchSeriesContentPayload) get(path string) gjson.Result {
	return gjson.GetBytes(p.raw, path)
}

// ID returns the requested series id.
// ID 返回请求的系列 ID。
func (p FetchSeriesContentPayload) ID() string {
	return p.id
}

// Raw returns the raw response body.
// Raw 返回响应体原始 JSON。
func (p FetchSeriesContentPayload) Raw() json.RawMessage {
	return p.raw
}

// Response returns the HTTP response.
// Response 返回 HTTP 响应。
func (p FetchSeriesContentPayload) Response() *http.Response {
	return p.resp
}

// Chapters returns an iterator over the chapter list of the current page.
// Chapters 返回当前页章节列表的迭代器。
func (p FetchSeriesContentPayload) Chapters() iter.Seq[ChapterInFetchSeriesContentPayload] {
	return func(yield func(ChapterInFetchSeriesContentPayload) bool) {
		p.get("page.seriesContents").ForEach(func(_, value gjson.Result) bool {
			return yield(ChapterInFetchSeriesContentPayload{payload: p, raw: json.RawMessage(value.Raw)})
		})
	}
}

// NextLastOrder returns the cursor for fetching the next page:
// the greatest chapter order on the current page, 0 when the page is empty.
// An empty page means the series has no more chapters.
// NextLastOrder 返回获取下一页所用的游标:当前页最大的章节次序,空页时为 0。
// 空页表示系列已无更多章节。
func (p FetchSeriesContentPayload) NextLastOrder() int64 {
	var max int64
	p.get("page.seriesContents").ForEach(func(_, value gjson.Result) bool {
		if v := value.Get("series.contentOrder").Int(); v > max {
			max = v
		}
		return true
	})
	return max
}

// ChapterInFetchSeriesContentPayload is a single chapter of a novel series
// chapter list.
// ChapterInFetchSeriesContentPayload 表示小说系列章节列表中的一个章节。
type ChapterInFetchSeriesContentPayload struct {
	payload FetchSeriesContentPayload
	raw     json.RawMessage
}

func (c ChapterInFetchSeriesContentPayload) get(path string) gjson.Result {
	return gjson.GetBytes(c.raw, path)
}

// Raw returns the raw chapter JSON.
// Raw 返回章节的原始 JSON。
func (c ChapterInFetchSeriesContentPayload) Raw() json.RawMessage {
	return c.raw
}

// ID returns the chapter's novel id.
// ID 返回章节的小说 ID。
func (c ChapterInFetchSeriesContentPayload) ID() string {
	return c.get("id").String()
}

// Order returns the chapter's position in the series (1-based).
// Order 返回章节在系列中的次序(从 1 开始)。
func (c ChapterInFetchSeriesContentPayload) Order() int64 {
	return c.get("series.contentOrder").Int()
}

// Title returns the chapter title.
// Title 返回章节标题。
func (c ChapterInFetchSeriesContentPayload) Title() string {
	return c.get("title").String()
}

// thumbnail returns the thumbnail entry matching this chapter's novel id,
// a zero result when absent.
// thumbnail 返回与本章节小说 ID 匹配的缩略图条目;缺失时为零值。
func (c ChapterInFetchSeriesContentPayload) thumbnail() gjson.Result {
	var ret gjson.Result
	c.payload.get("thumbnails.novel").ForEach(func(_, value gjson.Result) bool {
		if value.Get("id").String() == c.ID() {
			ret = value
			return false
		}
		return true
	})
	return ret
}

// URL returns the chapter cover URL from the thumbnail.
// URL 返回章节封面 URL(来自缩略图)。
func (c ChapterInFetchSeriesContentPayload) URL() string {
	return c.thumbnail().Get("url").String()
}

// AuthorID returns the author's unique identifier.
// AuthorID 返回作者唯一标识符。
func (c ChapterInFetchSeriesContentPayload) AuthorID() string {
	return c.thumbnail().Get("userId").String()
}

// AuthorName returns the author's display name.
// AuthorName 返回作者显示名称。
func (c ChapterInFetchSeriesContentPayload) AuthorName() string {
	return c.thumbnail().Get("userName").String()
}

// SeriesID returns the series id the chapter belongs to.
// SeriesID 返回章节所属系列的 ID。
func (c ChapterInFetchSeriesContentPayload) SeriesID() string {
	return c.thumbnail().Get("seriesId").String()
}

// SeriesTitle returns the series title the chapter belongs to.
// SeriesTitle 返回章节所属系列的标题。
func (c ChapterInFetchSeriesContentPayload) SeriesTitle() string {
	return c.thumbnail().Get("seriesTitle").String()
}

// Tags returns an iterator for chapter tags.
// Tags 返回章节标签的迭代器。
func (c ChapterInFetchSeriesContentPayload) Tags() iter.Seq[string] {
	return func(yield func(string) bool) {
		c.thumbnail().Get("tags").ForEach(func(_, value gjson.Result) bool {
			return yield(value.String())
		})
	}
}

// TextCount returns the chapter text length in characters.
// TextCount 返回章节文本长度(字符数)。
func (c ChapterInFetchSeriesContentPayload) TextCount() int64 {
	return c.thumbnail().Get("textCount").Int()
}

// WordCount returns the chapter word count.
// WordCount 返回章节字数。
func (c ChapterInFetchSeriesContentPayload) WordCount() int64 {
	return c.thumbnail().Get("wordCount").Int()
}

// ReadingTime returns the chapter reading time in minutes.
// ReadingTime 返回章节阅读时长(分钟)。
func (c ChapterInFetchSeriesContentPayload) ReadingTime() int64 {
	return c.thumbnail().Get("readingTime").Int()
}
