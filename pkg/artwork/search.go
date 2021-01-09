package artwork

import (
	"context"
	"net/url"
	"strconv"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/NateScarlet/pixiv/pkg/image"
	"github.com/NateScarlet/pixiv/pkg/user"
	"github.com/tidwall/gjson"
)

// SearchResult holds search data and provide useful methods.
type SearchResult struct {
	JSON gjson.Result
}

// ForEach iterates through values. skips advertisement container item.
func (r SearchResult) ForEach(iterator func(key gjson.Result, value gjson.Result) bool) {
	r.JSON.Get("illustManga.data").ForEach(func(key gjson.Result, value gjson.Result) bool {
		if value.Get("isAdContainer").Bool() {
			return true
		}
		return iterator(key, value)
	})

}

// Artworks appeared in search result.
func (r SearchResult) Artworks() []Artwork {
	ret := []Artwork{}
	r.ForEach(func(key, value gjson.Result) bool {
		a := Artwork{
			ID:    value.Get("id").String(),
			Title: value.Get("title").String(),
			Type:  value.Get("illustType").String(),
			Author: user.User{
				ID:   value.Get("userId").String(),
				Name: value.Get("userName").String(),
				Avatar: image.URLs{
					Mini: value.Get("profileImageUrl").String(),
				},
			},
			Description: value.Get("description").String(),
			Image: image.URLs{
				Thumb: value.Get("url").String(),
			},
			PageCount: value.Get("pageCount").Int(),
		}
		tagsData := value.Get("tags").Array()
		tags := make([]string, len(tagsData))
		for index, v := range tagsData {
			tags[index] = v.String()
		}
		a.Tags = tags
		ret = append(ret, a)
		return true
	})
	return ret

}

// SearchOptions for Search
// see SearchOption* functions for documention.
type SearchOptions struct {
	Page              int
	Order             string
	ContentRating     string
	Mode              string
	WidthLessThan     int
	WidthGreaterThan  int
	HeightLessThan    int
	HeightGreaterThan int
}

// SearchOption mutate SearchOptions
type SearchOption func(*SearchOptions)

// SearchOptionPage change page to retrive
func SearchOptionPage(page int) SearchOption {
	return func(so *SearchOptions) {
		so.Page = page
	}
}

// SearchOptionOrder change result order
// orders:
//  <empty string>
//    date descending (default)
//  date
//    date ascending
func SearchOptionOrder(order string) SearchOption {
	return func(so *SearchOptions) {
		so.Order = order
	}
}

// SearchOptionContentRating filter result by content rating
// ratings:
//   <empty string>
//     all content (default)
//   safe
//     general content
//   r18
//     restricted 18+ content
func SearchOptionContentRating(rating string) SearchOption {
	return func(so *SearchOptions) {
		so.ContentRating = rating
	}
}

// SearchOptionMode change search mode
// modes:
//   <empty string>
//     exact tag match (default)
//   s_tc
//     title or caption match
//   s_tag
//     partial tag match
func SearchOptionMode(mode string) SearchOption {
	return func(so *SearchOptions) {
		so.Mode = mode
	}
}

// SearchOptionResolution filter result by original image resolution,
// use 0 to unset limit.
func SearchOptionResolution(
	widthLessThan,
	widthGreaterThan,
	heightLessThan,
	heightGreaterThan int,
) SearchOption {
	return func(so *SearchOptions) {
		so.WidthLessThan = widthLessThan
		so.WidthGreaterThan = widthGreaterThan
		so.HeightLessThan = heightLessThan
		so.HeightGreaterThan = heightGreaterThan
	}
}

// Search calls pixiv artwork search api.
func Search(ctx context.Context, query string, opts ...SearchOption) (result SearchResult, err error) {
	var args = new(SearchOptions)
	for _, i := range opts {
		i(args)
	}

	if args.Page < 1 {
		args.Page = 1
	}

	q := url.Values{}
	if args.Page != 1 {
		q.Set("p", strconv.Itoa(args.Page))
	}
	if args.ContentRating != "" {
		q.Set("mode", args.ContentRating)
	}
	if args.Order != "" {
		q.Set("order", args.Order)
	}
	if args.Mode != "" {
		q.Set("s_mode", args.Mode)
	}
	if args.WidthLessThan > 1 {
		q.Set("wlt", strconv.Itoa(args.WidthLessThan))
	}
	if args.WidthGreaterThan > 1 {
		q.Set("wgt", strconv.Itoa(args.WidthGreaterThan))
	}
	if args.HeightLessThan > 1 {
		q.Set("hlt", strconv.Itoa(args.HeightLessThan))
	}
	if args.HeightGreaterThan > 1 {
		q.Set("hgt", strconv.Itoa(args.HeightGreaterThan))
	}

	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL(
		"/ajax/search/artworks/"+query,
		&q,
	).String())

	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := client.ParseAPIResult(resp.Body)
	if err != nil {
		return
	}
	result = SearchResult{
		JSON: body,
	}
	return
}
