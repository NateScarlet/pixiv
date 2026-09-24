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

// SearchV2 performs novel search and returns an immutable payload:
// known fields are exposed as methods, unknown fields are available from Raw.
// SearchV2 执行小说搜索,返回不可变记录:已知字段通过方法访问,未知字段通过 Raw 获取。
func SearchV2(ctx context.Context, query string, opts ...SearchV2Option) (_ SearchPayload, err error) {
	if query == "" {
		err = errors.New("pixiv: novel.SearchV2: query is required")
		return
	}
	o := newSearchV2Options(opts...)
	q := url.Values{}
	if o.page > 1 {
		q.Set("p", strconv.Itoa(o.page))
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/search/novels/"+url.PathEscape(query), &q).String())
	if err != nil {
		return
	}
	raw, err := client.ParseAPIResponseV2(resp)
	if err != nil {
		return
	}
	return SearchPayload{raw: raw, resp: resp}, nil
}

// SearchPayload is the immutable record of a novel search response.
// SearchPayload 是小说搜索响应的不可变记录。
type SearchPayload struct {
	raw  json.RawMessage
	resp *http.Response
}

// Raw returns the raw response body.
// Raw 返回响应体原始 JSON。
func (p SearchPayload) Raw() json.RawMessage {
	return p.raw
}

// Response returns the underlying HTTP response.
// Response 返回底层 HTTP 响应。
func (p SearchPayload) Response() *http.Response {
	return p.resp
}

// Items iterates through search result items.
// Items 遍历搜索结果条目。
func (p SearchPayload) Items() iter.Seq[ItemInSearchPayload] {
	return func(yield func(ItemInSearchPayload) bool) {
		gjson.GetBytes(p.raw, "novel.data").ForEach(func(_, value gjson.Result) bool {
			return yield(ItemInSearchPayload{raw: json.RawMessage(value.Raw)})
		})
	}
}

// ItemInSearchPayload is a single novel search result.
// ItemInSearchPayload 表示单个小说搜索结果。
type ItemInSearchPayload struct {
	raw json.RawMessage
}

func (i ItemInSearchPayload) get(path string) gjson.Result {
	return gjson.GetBytes(i.raw, path)
}

// Raw returns the raw item JSON.
// Raw 返回条目原始 JSON。
func (i ItemInSearchPayload) Raw() json.RawMessage {
	return i.raw
}

// ID returns the novel id.
// ID 返回小说 ID。
func (i ItemInSearchPayload) ID() string {
	return i.get("id").String()
}

// Title returns the novel title.
// Title 返回小说标题。
func (i ItemInSearchPayload) Title() string {
	return i.get("title").String()
}

// Description returns the novel description.
// Description 返回小说描述。
func (i ItemInSearchPayload) Description() string {
	return i.get("description").String()
}

// AuthorID returns the author's unique identifier.
// AuthorID 返回作者唯一标识符。
func (i ItemInSearchPayload) AuthorID() string {
	return i.get("userId").String()
}

// AuthorName returns the display name of the author.
// AuthorName 返回作者显示名称。
func (i ItemInSearchPayload) AuthorName() string {
	return i.get("userName").String()
}

// TextCount returns the total text length.
// TextCount 返回文本长度。
func (i ItemInSearchPayload) TextCount() int64 {
	return i.get("textCount").Int()
}

// BookmarkCount returns the number of bookmarks.
// BookmarkCount 返回收藏数。
func (i ItemInSearchPayload) BookmarkCount() int64 {
	return i.get("bookmarkCount").Int()
}

// Tags returns an iterator for novel tags.
// Tags 返回小说标签迭代器。
func (i ItemInSearchPayload) Tags() iter.Seq[string] {
	return func(yield func(string) bool) {
		i.get("tags").ForEach(func(_, v gjson.Result) bool {
			return yield(v.String())
		})
	}
}

// SeriesID returns the id of the series the novel belongs to, empty when not in a series.
// SeriesID 返回小说所属系列的 ID,不在系列时为空。
func (i ItemInSearchPayload) SeriesID() string {
	return i.get("seriesId").String()
}

// SeriesTitle returns the title of the series the novel belongs to, empty when not in a series.
// SeriesTitle 返回小说所属系列的标题,不在系列时为空。
func (i ItemInSearchPayload) SeriesTitle() string {
	return i.get("seriesTitle").String()
}

// SearchV2Option configures search parameters.
// SearchV2Option 配置搜索参数。
type SearchV2Option func(*SearchV2Options)

// SearchV2WithPage sets result page number.
// SearchV2WithPage 设置结果页码。
func SearchV2WithPage(page int) SearchV2Option {
	return func(o *SearchV2Options) {
		o.page = page
	}
}

// SearchV2Options search parameters.
// SearchV2Options 搜索参数。
type SearchV2Options struct {
	page int
}

func newSearchV2Options(opts ...SearchV2Option) *SearchV2Options {
	o := &SearchV2Options{page: 1}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
