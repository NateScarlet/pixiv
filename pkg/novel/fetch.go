package novel

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/tidwall/gjson"
)

// Fetch additional data from pixiv single novel api,
// returns an immutable payload: known fields are exposed as methods,
// unknown fields are available from Raw.
// Fetch 获取小说详情,返回不可变记录:已知字段通过方法访问,未知字段通过 Raw 获取。
func Fetch(ctx context.Context, id string) (_ FetchPayload, err error) {
	if id == "" {
		err = errors.New("pixiv: novel.Fetch: id is required")
		return
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/novel/"+id, nil).String())
	if err != nil {
		return
	}
	body, err := client.ParseAPIResponseV2(resp)
	if err != nil {
		return
	}
	return FetchPayload{id, body, resp}, nil
}

// FetchPayload is the immutable record of the novel detail response.
// FetchPayload 是小说详情响应的不可变记录。
type FetchPayload struct {
	id   string
	raw  json.RawMessage
	resp *http.Response
}

func (p FetchPayload) get(path string) gjson.Result {
	return gjson.GetBytes(p.raw, path)
}

// ID returns the novel id.
// ID 返回小说 ID。
func (p FetchPayload) ID() string {
	return p.id
}

// Raw returns the raw response body.
// Raw 返回响应体原始 JSON。
func (p FetchPayload) Raw() json.RawMessage {
	return p.raw
}

// Title returns the novel title.
// Title 返回小说标题。
func (p FetchPayload) Title() string {
	return p.get("title").String()
}

// Description returns the HTML formatted novel description.
// Description 返回 HTML 格式的小说描述。
func (p FetchPayload) Description() string {
	return p.get("description").String()
}

// Content returns the novel content.
// Content 返回小说内容。
func (p FetchPayload) Content() string {
	return p.get("content").String()
}

// CoverURL returns the novel cover URL.
// CoverURL 返回小说封面 URL。
func (p FetchPayload) CoverURL() string {
	return p.get("coverUrl").String()
}

// CreatedAt returns the creation time.
// CreatedAt 返回创建时间。
func (p FetchPayload) CreatedAt() time.Time {
	return p.get("createDate").Time()
}

// UploadedAt returns the upload time.
// UploadedAt 返回上传时间。
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

// Tags returns an iterator for novel tags.
// Tags 返回小说标签迭代器。
func (p FetchPayload) Tags() iter.Seq[string] {
	return func(yield func(string) bool) {
		p.get("tags.tags.#.tag").ForEach(func(_, value gjson.Result) bool {
			return yield(value.String())
		})
	}
}

// PageCount returns the total number of pages.
// PageCount 返回总页数。
func (p FetchPayload) PageCount() int64 {
	return p.get("pageCount").Int()
}

// TextCount returns the total text length.
// TextCount 返回文本长度。
func (p FetchPayload) TextCount() int64 {
	return p.get("textCount").Int()
}

// CommentCount returns the number of comments.
// CommentCount 返回回复数。
func (p FetchPayload) CommentCount() int64 {
	return p.get("commentCount").Int()
}

// LikeCount returns the number of likes.
// LikeCount 返回赞数。
func (p FetchPayload) LikeCount() int64 {
	return p.get("likeCount").Int()
}

// ViewCount returns the number of views.
// ViewCount 返回浏览量。
func (p FetchPayload) ViewCount() int64 {
	return p.get("viewCount").Int()
}

// BookmarkCount returns the number of bookmarks.
// BookmarkCount 返回收藏数。
func (p FetchPayload) BookmarkCount() int64 {
	return p.get("bookmarkCount").Int()
}

// CreationMethod returns the novel's creation method.
// CreationMethod 返回小说的创作方式。
func (p FetchPayload) CreationMethod() CreationMethod {
	return parseCreationMethod(p.get("aiType"))
}

// Series returns the series the novel belongs to,
// zero value (IsZero) when the novel is not in any series.
// Series 返回小说所属的系列信息;小说不属于任何系列时返回零值记录。
func (p FetchPayload) Series() FetchPayloadSeries {
	return FetchPayloadSeries{raw: json.RawMessage(p.get("seriesNavData").Raw)}
}

// FetchPayloadSeries is the position of the novel inside its series,
// parsed from seriesNavData.
// FetchPayloadSeries 表示小说在所属系列中的位置,由 seriesNavData 解析。
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

// Order returns the position of the novel in the series (1-based).
// Order 返回小说在系列中的次序(从 1 开始)。
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

// IsZero reports whether the novel is not in any series.
// IsZero 报告小说是否不属于任何系列。
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

// ID returns the adjacent chapter's novel id.
// ID 返回相邻章节的小说 ID。
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
