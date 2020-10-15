package artwork

import (
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

// SearchWithClient do search with given client.
func SearchWithClient(c client.Client, query string, page int) (result SearchResult, err error) {
	resp, err := c.Get(c.EndpointURL(
		"/ajax/search/artworks/"+query,
		&url.Values{
			"p": []string{strconv.Itoa(page)},
		},
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

// Search calls pixiv artwork search api.
func Search(query string, page int) (result SearchResult, err error) {
	return SearchWithClient(*client.Default, query, page)
}
