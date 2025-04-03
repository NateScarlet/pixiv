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

// SearchV2 performs artwork search
// SearchV2 执行作品搜索
func SearchV2(ctx context.Context, query string, options ...SearchV2Option) (_ SearchPayload, err error) {
	if query == "" {
		return SearchPayload{}, errors.New("search query required")
	}

	opts := newSearchV2Options(options...)
	q := make(url.Values)

	if opts.page > 1 {
		q.Set("p", strconv.Itoa(opts.page))
	}
	if opts.contentRating != "" {
		q.Set("mode", string(opts.contentRating))
	}
	if opts.order != "" {
		q.Set("order", string(opts.order))
	}
	if opts.mode != "" {
		q.Set("s_mode", string(opts.mode))
	}
	if opts.widthLt > 0 {
		q.Set("wlt", strconv.FormatInt(opts.widthLt, 10))
	}
	if opts.widthGt > 0 {
		q.Set("wgt", strconv.FormatInt(opts.widthGt, 10))
	}
	if opts.heightLt > 0 {
		q.Set("hlt", strconv.FormatInt(opts.heightLt, 10))
	}
	if opts.heightGt > 0 {
		q.Set("hgt", strconv.FormatInt(opts.heightGt, 10))
	}

	c := client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/search/artworks/"+url.PathEscape(query), &q).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()

	raw, err := client.ParseAPIResponse(resp.Body)
	if err != nil {
		return
	}

	return SearchPayload{
		raw:  raw,
		resp: resp,
	}, nil
}

// SearchPayload contains search result data
// SearchPayload 包含搜索结果数据
type SearchPayload struct {
	raw  json.RawMessage
	resp *http.Response
}

func (p SearchPayload) Raw() json.RawMessage {
	return p.raw
}

func (p SearchPayload) Response() *http.Response {
	return p.resp
}

// Items iterates through search results
// Items 遍历搜索结果条目
func (p SearchPayload) Items() iter.Seq[ItemInSearchPayload] {
	return func(yield func(ItemInSearchPayload) bool) {
		result := gjson.GetBytes(p.raw, "illustManga.data")
		result.ForEach(func(_, value gjson.Result) bool {
			if value.Get("isAdContainer").Bool() {
				return true // Skip ads
			}
			return yield(ItemInSearchPayload{raw: json.RawMessage(value.Raw)})
		})
	}
}

// ItemInSearchPayload represents single search result
// ItemInSearchPayload 表示单个搜索结果
type ItemInSearchPayload struct {
	raw json.RawMessage
}

func (p ItemInSearchPayload) Raw() json.RawMessage {
	return p.raw
}

func (i ItemInSearchPayload) get(path string) gjson.Result {
	return gjson.GetBytes(i.raw, path)
}

// ID returns artwork identifier
// ID 返回作品ID
func (i ItemInSearchPayload) ID() string {
	return i.get("id").String()
}

// Title returns artwork title
// Title 返回作品标题
func (i ItemInSearchPayload) Title() string {
	return i.get("title").String()
}

// Type returns artwork content type
// Type 返回作品内容类型
func (i ItemInSearchPayload) Type() ContentType {
	return parseContentType(i.get("illustType"))
}

// Description returns HTML formatted description
// Description 返回HTML格式描述
func (i ItemInSearchPayload) Description() string {
	return i.get("description").String()
}

// MaxWidth250URL returns the thumbnail URL (max 250px width).
// MaxWidth250URL 返回最大250px宽度的缩略图URL。
func (i ItemInSearchPayload) MaxWith250URL() string {
	return i.get("url").String()
}

// AuthorID returns creator's user ID
// AuthorID 返回作者用户ID
func (i ItemInSearchPayload) AuthorID() string {
	return i.get("userId").String()
}

// AuthorName returns creator's display name
// AuthorName 返回作者显示名称
func (i ItemInSearchPayload) AuthorName() string {
	return i.get("userName").String()
}

// AuthorProfileImageURL returns creator's profile image URL
// AuthorProfileImageURL 返回作者头像URL
func (i ItemInSearchPayload) AuthorProfileImageURL() string {
	return i.get("profileImageUrl").String()
}

// PageCount returns total number of pages
// PageCount 返回作品总页数
func (i ItemInSearchPayload) PageCount() int {
	return int(i.get("pageCount").Int())
}

// Tags returns associated tags
// Tags 返回关联标签
func (i ItemInSearchPayload) Tags() iter.Seq[string] {
	return func(yield func(string) bool) {
		i.get("tags").ForEach(func(_, v gjson.Result) bool {
			return yield(v.String())
		})
	}
}

// SearchOption configures search parameters
// SearchOption 配置搜索参数
type SearchV2Option func(*SearchV2Options)

// SearchWithPage sets result page number
// SearchWithPage 设置结果页码
func SearchWithPage(page int) SearchV2Option {
	return func(so *SearchV2Options) {
		so.page = page
	}
}

// SearchWithOrder sets sorting order
// SearchWithOrder 设置排序方式
func SearchWithOrder(order Order) SearchV2Option {
	return func(so *SearchV2Options) {
		so.order = order
	}
}

// SearchWithContentRating filters by content rating
// SearchWithContentRating 按内容分级筛选
func SearchWithContentRating(rating ContentRating) SearchV2Option {
	return func(so *SearchV2Options) {
		so.contentRating = rating
	}
}

// SearchWithMode sets search matching mode
// SearchWithMode 设置搜索匹配模式
func SearchWithMode(mode SearchMode) SearchV2Option {
	return func(so *SearchV2Options) {
		so.mode = mode
	}
}

// SearchWithResolution filters by image dimensions
// SearchWithResolution 按图像尺寸筛选
func SearchWithResolution(minWidth, maxWidth, minHeight, maxHeight int64) SearchV2Option {
	return func(so *SearchV2Options) {
		so.widthGt = minWidth
		so.widthLt = maxWidth
		so.heightGt = minHeight
		so.heightLt = maxHeight
	}
}

type SearchV2Options struct {
	page          int
	order         Order
	contentRating ContentRating
	mode          SearchMode
	widthGt       int64
	widthLt       int64
	heightGt      int64
	heightLt      int64
}

func newSearchV2Options(opts ...SearchV2Option) *SearchV2Options {
	o := &SearchV2Options{page: 1}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// ContentRating defines artwork content classification
// ContentRating 定义作品内容分级
type ContentRating string

const (
	AllContent  ContentRating = ""     // All content types | 全部内容类型
	SafeContent ContentRating = "safe" // Suitable for all ages | 全年龄适宜
	R18Content  ContentRating = "r18"  // Restricted adult content | 限制级内容
)

var (
	// Deprecated: use [AllContent] instead
	ContentRatingAll = AllContent
	// Deprecated: use [SafeContent] instead
	ContentRatingSafe = SafeContent
	// Deprecated: use [R18Content] instead
	ContentRatingR18 = R18Content
)

// Order defines sorting criteria
// Order 定义排序方式
type Order string

const (
	DateDescending Order = ""     // Newest first (default) | 按时间降序（默认）
	DateAscending  Order = "date" // Oldest first | 按时间升序
)

var (
	// Deprecated: use [DateDescending] instead
	OrderDateDSC = DateDescending
	// Deprecated: use [DateAscending] instead
	OrderDateASC = DateAscending
)

// SearchMode defines artwork search matching strategy
// SearchMode 定义作品搜索匹配策略
type SearchMode string

const (
	// TagSearch performs exact tag matching (default)
	// TagSearch 执行精确标签匹配（默认模式）
	TagSearch SearchMode = ""

	// PartialTagSearch matches partial tag content
	// PartialTagSearch 匹配标签部分内容
	PartialTagSearch SearchMode = "s_tag"

	// TitleOrCaptionSearch matches title or description
	// TitleOrCaptionSearch 匹配标题或作品描述
	TitleOrCaptionSearch SearchMode = "s_tc"
)

var (
	// Deprecated: use [TagSearch] instead
	// exact tag match (default)
	SearchModeTag = TagSearch

	// Deprecated: use [PartialTagSearch] instead
	// partial tag match
	SearchModePartialTag = PartialTagSearch

	// Deprecated: use [TitleOrCaptionSearch] instead
	// title or caption match
	SearchModeTitleOrCaption = TitleOrCaptionSearch
)
