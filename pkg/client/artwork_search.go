package client

import (
	"github.com/tidwall/gjson"
)

// ArtworkSearchResult holds search data and provide useful methods.
type ArtworkSearchResult struct {
	JSON gjson.Result
}

// ForEach iterates through values. skips advertisement container item.
func (r ArtworkSearchResult) ForEach(iterator func(key gjson.Result, value gjson.Result) bool) {
	r.JSON.Get("illustManga.data").ForEach(func(key gjson.Result, value gjson.Result) bool {
		if value.Get("isAdContainer").Bool() {
			return true
		}
		return iterator(key, value)
	})

}

// Artworks appeared in search result.
func (r ArtworkSearchResult) Artworks() []Artwork {
	ret := []Artwork{}
	r.ForEach(func(key, value gjson.Result) bool {
		a := Artwork{
			ID:    value.Get("illustId").String(),
			Title: value.Get("illustTitle").String(),
			Type:  value.Get("illustType").String(),
			Author: User{
				ID:   value.Get("userId").String(),
				Name: value.Get("userName").String(),
				AvatarURLs: ImageURLs{
					Mini: value.Get("profileImageUrl").String(),
				},
			},
			Description: value.Get("description").String(),
			URLs: ImageURLs{
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

// SearchArtwork calls pixiv artwork search api.
func SearchArtwork(query string, page int) (result ArtworkSearchResult, err error) {
	u := APIArtworkSearchURL(query, page)
	resp, err := httpGetBytes(u.String())
	if err != nil {
		return
	}

	payload := gjson.ParseBytes(resp)
	err = validateAPIPayload(payload)
	result = ArtworkSearchResult{
		JSON: payload.Get("body"),
	}
	return
}
