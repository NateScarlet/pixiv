package novel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/tidwall/gjson"
)

// FetchSeries fetches a novel series' metadata,
// returns an immutable payload: known fields are exposed as methods,
// unknown fields are available from Raw.
// Fetch 获取小说系列的元数据,返回不可变记录:已知字段通过方法访问,未知字段通过 Raw 获取。
func FetchSeries(ctx context.Context, id string) (_ FetchSeriesPayload, err error) {
	if id == "" {
		err = errors.New("pixiv: novel.FetchSeries: id is required")
		return
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/novel/series/"+id, nil).String())
	if err != nil {
		return
	}
	body, err := client.ParseAPIResponseV2(resp)
	if err != nil {
		return
	}
	return FetchSeriesPayload{id, body, resp}, nil
}

// FetchSeriesPayload is the immutable record of a novel series metadata response.
// FetchSeriesPayload 是小说系列元数据响应的不可变记录。
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

// Title returns the series title.
// Title 返回系列标题。
func (p FetchSeriesPayload) Title() string {
	return p.get("title").String()
}

// Total returns the total number of chapters in the series.
// Total 返回系列的章节总数。
func (p FetchSeriesPayload) Total() int64 {
	return p.get("total").Int()
}

// FirstNovelID returns the first chapter's novel id.
// FirstNovelID 返回首章的小说 ID。
func (p FetchSeriesPayload) FirstNovelID() string {
	return p.get("firstNovelId").String()
}

// LatestNovelID returns the latest chapter's novel id.
// LatestNovelID 返回最新一章的小说 ID。
func (p FetchSeriesPayload) LatestNovelID() string {
	return p.get("latestNovelId").String()
}

// CoverURL returns the series cover URL.
// CoverURL 返回系列封面 URL。
func (p FetchSeriesPayload) CoverURL() string {
	return p.get("cover.urls.original").String()
}
