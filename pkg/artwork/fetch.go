package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/url"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/tidwall/gjson"
)

func Fetch(ctx context.Context, id string) (_ FetchPayload, err error) {
	if id == "" {
		err = errors.New("pixiv: artwork.Fetch: id is required")
		return
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/illust/"+id, nil).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := client.ParseAPIResponse(resp.Body)
	if err != nil {
		return
	}
	return FetchPayload{id, body, resp}, nil
}

type FetchPayload struct {
	id   string
	raw  json.RawMessage
	resp *http.Response
}

func (p FetchPayload) ID() string {
	return p.id
}

// URL to view artwork web page.
func (p FetchPayload) URL() *url.URL {
	var base = p.resp.Request.URL
	return &url.URL{
		Scheme: base.Scheme,
		Host:   base.Host,
		Path:   "/artworks/" + p.ID(),
	}
}

func (p FetchPayload) Response() *http.Response {
	return p.resp
}

func (p FetchPayload) Raw() json.RawMessage {
	return p.raw
}

func (p FetchPayload) get(path string) gjson.Result {
	return gjson.GetBytes(p.raw, path)
}

// Title returns the artwork title.
// Title 返回作品标题。
func (p FetchPayload) Title() string {
	return p.get("illustTitle").String()
}

// ContentType defines the classification of artwork on Pixiv
// ContentType 定义Pixiv作品内容类型
type ContentType int

const (
	UnknownContentType ContentType = iota // Unknown or unsupported type | 未知或不支持的类型
	Illust                                // Static illustration | 静态插画
	Manga                                 // Multi-page work | 多页漫画作品
	Animation                             // Animated work | 动态作品
)

// Type returns the artwork content type classification
// Type 返回作品内容类型分类
func (p FetchPayload) Type() ContentType {
	return parseContentType(p.get("illustType"))
}

func parseContentType(v gjson.Result) ContentType {
	switch v.Int() {
	case 0:
		return Illust
	case 1:
		return Manga
	case 2:
		return Animation
	default:
		return UnknownContentType
	}
}

// Description returns the HTML formatted artwork description.
// Description 返回HTML格式的作品描述。
func (p FetchPayload) Description() string {
	return p.get("description").String()
}

// CreatedAt returns the creation time.
// CreatedAt 返回作品创建时间。
func (p FetchPayload) CreatedAt() time.Time {
	return p.get("createDate").Time()
}

// UploadedAt returns the upload time.
// UploadedAt 返回作品上传时间。
func (p FetchPayload) UploadedAt() time.Time {
	return p.get("uploadDate").Time()
}

// AuthorID returns the author's unique identifier.
// AuthorID 返回作者唯一标识符。
func (p FetchPayload) AuthorID() string {
	return p.get("userId").String()
}

// AuthorName returns the display name of the author.
// AuthorName 返回作者显示名称。
func (p FetchPayload) AuthorName() string {
	return p.get("userName").String()
}

// Width returns the artwork width in pixels.
// Width 返回作品宽度（像素）。
func (p FetchPayload) Width() int64 {
	return p.get("width").Int()
}

// Height returns the artwork height in pixels.
// Height 返回作品高度（像素）。
func (p FetchPayload) Height() int64 {
	return p.get("height").Int()
}

// PageCount returns the total number of pages.
// PageCount 返回作品总页数。
func (p FetchPayload) PageCount() int64 {
	return p.get("pageCount").Int()
}

// CommentCount returns the number of comments.
// CommentCount 返回作品评论数。
func (p FetchPayload) CommentCount() int64 {
	return p.get("commentCount").Int()
}

// LikeCount returns the number of likes.
// LikeCount 返回作品点赞数。
func (p FetchPayload) LikeCount() int64 {
	return p.get("likeCount").Int()
}

// ViewCount returns the view count.
// ViewCount 返回作品浏览数。
func (p FetchPayload) ViewCount() int64 {
	return p.get("viewCount").Int()
}

// BookmarkCount returns the bookmark count.
// BookmarkCount 返回作品收藏数。
func (p FetchPayload) BookmarkCount() int64 {
	return p.get("bookmarkCount").Int()
}

// MaxWidth48URL returns the thumbnail URL (max 48px width).
// MaxWidth48URL 返回最大48px宽度的缩略图URL。
func (p FetchPayload) MaxWidth48URL() string {
	return p.get("urls.mini").String()
}

// MaxWidth250URL returns the thumbnail URL (max 250px width).
// MaxWidth250URL 返回最大250px宽度的缩略图URL。
func (p FetchPayload) MaxWidth250URL() string {
	return p.get("urls.thumb").String()
}

// MaxWidth540URL returns the medium-sized image URL (max 540px width).
// MaxWidth540URL 返回最大540px宽度的中等尺寸图片URL。
func (p FetchPayload) MaxWidth540URL() string {
	return p.get("urls.small").String()
}

// MaxWidth1200URL returns the large image URL (max 1200px width).
// MaxWidth1200URL 返回最大1200px宽度的大图URL。
func (p FetchPayload) MaxWidth1200URL() string {
	return p.get("urls.regular").String()
}

// OriginalURL returns the original resolution image URL.
// OriginalURL 返回原始分辨率的图片URL。
func (p FetchPayload) OriginalURL() string {
	return p.get("urls.original").String()
}

// Tags returns an iterator for artwork tags.
// Tags 返回作品标签的迭代器。
func (p FetchPayload) Tags() iter.Seq[string] {
	return func(yield func(string) bool) {
		p.get("tags.tags.#.tag").ForEach(func(_, value gjson.Result) bool {
			return yield(value.String())
		})
	}
}

// CreationMethod indicates the artwork's creation method
// CreationMethod 表示作品的创作方式
type CreationMethod int

const (
	UnknownCreationMethod CreationMethod = iota // Unknown creation method | 未知创作类型
	ManuallyCreated                             // Created without AI tools | 非AI生成作品
	AIGenerated                                 // Created with AI technology | AI生成作品
)

// CreationMethod returns the artwork's creation classification
// CreationMethod 返回作品的创作方式分类
func (p FetchPayload) CreationMethod() CreationMethod {
	switch val := p.get("aiType").Int(); val {
	case 1:
		return ManuallyCreated
	case 2:
		return AIGenerated
	default: // 包括0和其他未定义值
		return UnknownCreationMethod
	}
}

// Series returns the series the artwork belongs to,
// zero value (IsZero) when the artwork is not in any series.
// Series 返回作品所属的系列信息;作品不属于任何系列时返回零值记录。
func (p FetchPayload) Series() FetchPayloadSeries {
	return FetchPayloadSeries{raw: json.RawMessage(p.get("seriesNavData").Raw)}
}

// FetchPayloadSeries is the position of the artwork inside its series,
// parsed from seriesNavData.
// FetchPayloadSeries 表示作品在所属系列中的位置,由 seriesNavData 解析。
type FetchPayloadSeries struct {
	raw json.RawMessage
}

func (s FetchPayloadSeries) get(path string) gjson.Result {
	return gjson.GetBytes(s.raw, path)
}

// ID returns the series id.
// ID 返回系列 ID。
func (s FetchPayloadSeries) ID() string {
	return s.get("seriesId").String()
}

// Title returns the series title.
// Title 返回系列标题。
func (s FetchPayloadSeries) Title() string {
	return s.get("title").String()
}

// Order returns the position of the artwork in the series (1-based).
// Order 返回作品在系列中的次序(从 1 开始)。
func (s FetchPayloadSeries) Order() int64 {
	return s.get("order").Int()
}

// Prev returns the previous chapter in the series, zero value when absent.
// Prev 返回系列中紧邻的上一章节;无上一章节时为零值记录。
func (s FetchPayloadSeries) Prev() SeriesNeighbor {
	return SeriesNeighbor{raw: json.RawMessage(s.get("prev").Raw)}
}

// Next returns the next chapter in the series, zero value when absent.
// Next 返回系列中紧邻的下一章节;无下一章节时为零值记录。
func (s FetchPayloadSeries) Next() SeriesNeighbor {
	return SeriesNeighbor{raw: json.RawMessage(s.get("next").Raw)}
}

// Raw returns the raw seriesNavData JSON.
// Raw 返回 seriesNavData 的原始 JSON。
func (s FetchPayloadSeries) Raw() json.RawMessage {
	return s.raw
}

// IsZero reports whether the artwork is not in any series.
// IsZero 报告作品是否不属于任何系列。
func (s FetchPayloadSeries) IsZero() bool {
	return s.ID() == ""
}

// SeriesNeighbor is an adjacent chapter of the series.
// SeriesNeighbor 表示系列中相邻的一个章节。
type SeriesNeighbor struct {
	raw json.RawMessage
}

func (n SeriesNeighbor) get(path string) gjson.Result {
	return gjson.GetBytes(n.raw, path)
}

// ID returns the adjacent chapter's artwork id.
// ID 返回相邻章节的作品 ID。
func (n SeriesNeighbor) ID() string {
	return n.get("id").String()
}

// Title returns the adjacent chapter's title.
// Title 返回相邻章节的标题。
func (n SeriesNeighbor) Title() string {
	return n.get("title").String()
}

// Order returns the position of the adjacent chapter in the series.
// Order 返回相邻章节在系列中的次序。
func (n SeriesNeighbor) Order() int64 {
	return n.get("order").Int()
}

// Available reports whether the adjacent chapter is accessible.
// Available 报告相邻章节是否可访问。
func (n SeriesNeighbor) Available() bool {
	return n.get("available").Bool()
}

// Raw returns the raw prev/next JSON.
// Raw 返回相邻章节的原始 JSON。
func (n SeriesNeighbor) Raw() json.RawMessage {
	return n.raw
}
