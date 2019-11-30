package client

import "github.com/tidwall/gjson"

// NovelSearchResult holds search data and provide useful methods.
type NovelSearchResult struct {
	JSON gjson.Result
}

// ForEach iterates through novel data items.
func (r NovelSearchResult) ForEach(iterator func(key, value gjson.Result) bool) {
	r.JSON.Get("novel.data").ForEach(iterator)

}

// Novels appeared in the search result.
func (r NovelSearchResult) Novels() []Novel {
	ret := []Novel{}
	r.ForEach(func(key, value gjson.Result) bool {
		n := Novel{
			ID:          value.Get("id").String(),
			Title:       value.Get("title").String(),
			Description: value.Get("Description").String(),
			Author: User{
				ID:   value.Get("userId").String(),
				Name: value.Get("userName").String(),
			},
			TextCount:     value.Get("textCount").Int(),
			BookmarkCount: value.Get("bookmarkCount").Int(),
			Series: NovelSeries{
				ID:    value.Get("seriesId").String(),
				Title: value.Get("seriesTitle").String(),
			},
		}
		tagsData := value.Get("tags").Array()
		tags := make([]string, len(tagsData))
		for index, v := range tagsData {
			tags[index] = v.String()
		}
		n.Tags = tags

		ret = append(
			ret,
			n,
		)
		return true
	})
	return ret

}

// SearchNovel calls pixiv novel search api.
func SearchNovel(query string, page int) (result NovelSearchResult, err error) {
	u := APINovelSearchURL(query, page)
	resp, err := httpGetBytes(u.String())
	if err != nil {
		return
	}

	payload := gjson.ParseBytes(resp)
	err = validateAPIPayload(payload)
	result = NovelSearchResult{
		JSON: payload.Get("body"),
	}
	return
}
