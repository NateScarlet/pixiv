package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/tidwall/gjson"
)

// RankMode defines ranking list type
// RankMode 定义排行榜类型
type RankMode string

const (
	DailyRank     RankMode = "daily"
	WeeklyRank    RankMode = "weekly"
	MonthlyRank   RankMode = "monthly"
	RookieRank    RankMode = "rookie"
	OriginalRank  RankMode = "original"
	MaleRank      RankMode = "male"
	FemaleRank    RankMode = "female"
	DailyR18Rank  RankMode = "daily_r18"
	WeeklyR18Rank RankMode = "weekly_r18"
	MaleR18Rank   RankMode = "male_r18"
	FemaleR18Rank RankMode = "female_r18"
	R18GRank      RankMode = "r18g"
)

func (t ContentType) rankInput() string {
	switch t {
	case Illust:
		return "illust"
	case Animation:
		return "ugoira"
	case Manga:
		return "manga"
	default:
		return ""
	}
}

// FetchRank fetches ranking list with specified criteria
// FetchRank 获取指定条件的排行榜数据
func FetchRank(ctx context.Context, mode RankMode, options ...FetchRankOption) (_ FetchRankPayload, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("pixiv: artwork.FetchRank(%q): %w", mode, err)
		}
	}()

	if mode == "" {
		return FetchRankPayload{}, errors.New("mode required")
	}

	opts := newFetchRankOptions(options...)
	q := make(url.Values)
	q.Set("mode", string(mode))

	if opts.content != UnknownContentType {
		var s = opts.content.rankInput()
		if s == "" {
			err = fmt.Errorf("unsupported content type %q", opts.content)
			return
		}
		q.Set("content", s)
	}
	if !opts.date.IsZero() {
		q.Set("date", opts.date.Format("20060102"))
	}
	if opts.page > 1 {
		q.Set("p", strconv.Itoa(opts.page))
	}
	q.Set("format", "json")

	c := client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ranking.php", &q).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	return FetchRankPayload{
		raw:  raw,
		resp: resp,
	}, nil
}

func newFetchRankOptions(options ...FetchRankOption) *FetchRankOptions {
	var opts = new(FetchRankOptions)
	for _, i := range options {
		i(opts)
	}
	return opts
}

type FetchRankOptions struct {
	content ContentType
	date    time.Time
	page    int
}

type FetchRankOption func(*FetchRankOptions)

func FetchRankWithContentType(contentType ContentType) FetchRankOption {
	return func(opts *FetchRankOptions) {
		opts.content = contentType
	}
}

func FetchRankWithDate(date time.Time) FetchRankOption {
	return func(opts *FetchRankOptions) {
		opts.date = date
	}
}

func FetchRankWithPage(page int) FetchRankOption {
	return func(opts *FetchRankOptions) {
		opts.page = page
	}
}

// FetchRankPayload contains raw ranking data
// FetchRankPayload 包含原始排行榜数据
type FetchRankPayload struct {
	raw  json.RawMessage
	resp *http.Response
}

// URL returns the web page URL for this ranking list
// URL 返回该排行榜对应的网页URL
func (p FetchRankPayload) URL() *url.URL {
	// 复制原始URL
	u := *p.resp.Request.URL

	// 获取查询参数并移除format
	q := u.Query()
	q.Del("format")
	u.RawQuery = q.Encode()
	return &u
}

// Raw returns original JSON data
// Raw 返回原始JSON数据
func (p FetchRankPayload) Raw() json.RawMessage {
	return p.raw
}

// Response returns HTTP response metadata
// Response 返回HTTP响应元数据
func (p FetchRankPayload) Response() *http.Response {
	return p.resp
}

// Items iterates through ranking entries
// Items 遍历排行榜条目
func (p FetchRankPayload) Items() iter.Seq[ItemInFetchRankPayload] {
	return func(yield func(ItemInFetchRankPayload) bool) {
		gjson.GetBytes(p.raw, "contents").ForEach(func(_, value gjson.Result) bool {
			return yield(ItemInFetchRankPayload{json.RawMessage(value.Raw)})
		})
	}
}

// ItemInFetchRankPayload represents single entry in ranking
// ItemInFetchRankPayload 表示排行榜单个条目
type ItemInFetchRankPayload struct {
	raw json.RawMessage
}

func (p ItemInFetchRankPayload) Raw() json.RawMessage {
	return p.raw
}

func (i ItemInFetchRankPayload) get(path string) gjson.Result {
	return gjson.GetBytes(i.raw, path)
}

// ID returns artwork identifier
// ID 返回作品ID
func (i ItemInFetchRankPayload) ID() string {
	return i.get("illust_id").String()
}

// Title returns artwork title
// Title 返回作品标题
func (i ItemInFetchRankPayload) Title() string {
	return i.get("title").String()
}

// Type returns artwork content type
// Type 返回作品内容类型
func (i ItemInFetchRankPayload) Type() ContentType {
	return parseContentType(i.get("illust_type"))
}

// Width returns artwork width in pixels
// Width 返回作品宽度（像素）
func (i ItemInFetchRankPayload) Width() int64 {
	return i.get("width").Int()
}

// Height returns artwork height in pixels
// Height 返回作品高度（像素）
func (i ItemInFetchRankPayload) Height() int64 {
	return i.get("height").Int()
}

// UploadedAt returns artwork upload timestamp
// UploadedAt 返回作品上传时间
func (i ItemInFetchRankPayload) UploadedAt() time.Time {
	return time.Unix(i.get("illust_upload_timestamp").Int(), 0)
}

// AuthorID returns author identifier
// AuthorID 返回作者ID
func (i ItemInFetchRankPayload) AuthorID() string {
	return i.get("user_id").String()
}

// AuthorName returns author display name
// AuthorName 返回作者显示名称
func (i ItemInFetchRankPayload) AuthorName() string {
	return i.get("user_name").String()
}

// AuthorProfileImageURL returns author profile image URL
// AuthorProfileImageURL 返回作者头像URL
func (i ItemInFetchRankPayload) AuthorProfileImageURL() string {
	return i.get("profile_img").String()
}

// Position returns current ranking position
// Position 返回当前排名
func (i ItemInFetchRankPayload) Position() int {
	return int(i.get("rank").Int())
}

// PreviousDayPosition returns previous day's ranking position
// PreviousDayPosition 返回昨日排名
func (i ItemInFetchRankPayload) PreviousDayPosition() int {
	return int(i.get("yes_rank").Int())
}

// PageCount returns total number of pages
// PageCount 返回作品总页数
func (i ItemInFetchRankPayload) PageCount() int {
	return int(i.get("illust_page_count").Int())
}

// Tags returns artwork tags
// Tags 返回作品标签
func (i ItemInFetchRankPayload) Tags() iter.Seq[string] {
	return func(yield func(string) bool) {
		i.get("tags").ForEach(func(_, v gjson.Result) bool {
			return yield(v.String())
		})
	}
}

// MaxWidth1200URL returns the large image URL (max 1200px width).
// MaxWidth1200URL 返回最大1200px宽度的大图URL。
func (i ItemInFetchRankPayload) MaxWidth1200URL() string {
	return i.get("url").String()
}
