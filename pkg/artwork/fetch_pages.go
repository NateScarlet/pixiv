package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/tidwall/gjson"
)

func FetchPages(ctx context.Context, id string) (_ FetchPagesPayload, err error) {
	if id == "" {
		err = errors.New("pixiv: artwork.FetchPages: id is required")
		return
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/illust/"+id+"/pages", nil).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := client.ParseAPIResponse(resp.Body)
	if err != nil {
		return
	}
	return FetchPagesPayload{id, body, resp}, nil
}

type FetchPagesPayload struct {
	id   string
	raw  json.RawMessage
	resp *http.Response
}

func (p FetchPagesPayload) ID() string {
	return p.id
}

func (p FetchPagesPayload) Raw() json.RawMessage {
	return p.raw
}

func (p FetchPagesPayload) Resp() *http.Response {
	return p.resp
}

func (p FetchPagesPayload) Pages() iter.Seq[PageInFetchPagesPayload] {
	return func(yield func(PageInFetchPagesPayload) bool) {
		gjson.ParseBytes(p.raw).ForEach(func(key, value gjson.Result) bool {
			return yield(PageInFetchPagesPayload{json.RawMessage(value.Raw)})
		})
	}
}

type PageInFetchPagesPayload struct {
	raw json.RawMessage
}

func (p PageInFetchPagesPayload) get(path string) gjson.Result {
	return gjson.GetBytes(p.raw, path)
}

// MaxWidth128URL returns the thumbnail URL with max 128px width.
// MaxWidth128URL 返回最大128px宽度的缩略图URL。
func (p PageInFetchPagesPayload) MaxWidth128URL() string {
	return p.get("urls.thumb_mini").String()
}

// MaxWidth540URL returns the medium-sized image URL with max 540px width.
// MaxWidth540URL 返回最大540px宽度的中等尺寸图片URL。
func (p PageInFetchPagesPayload) MaxWidth540URL() string {
	return p.get("urls.small").String()
}

// MaxWidth1200URL returns the large image URL with max 1200px width.
// MaxWidth1200URL 返回最大1200px宽度的大图URL。
func (p PageInFetchPagesPayload) MaxWidth1200URL() string {
	return p.get("urls.regular").String()
}

// OriginalURL returns the original resolution image URL.
// OriginalURL 返回原始分辨率的图片URL。
func (p PageInFetchPagesPayload) OriginalURL() string {
	return p.get("urls.original").String()
}

// Width returns the pixel width of the page.
// Width 返回页面的像素宽度。
func (p PageInFetchPagesPayload) Width() int64 {
	return p.get("width").Int()
}

// Height returns the pixel height of the page.
// Height 返回页面的像素高度。
func (p PageInFetchPagesPayload) Height() int64 {
	return p.get("height").Int()
}
