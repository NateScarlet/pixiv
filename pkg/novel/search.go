package novel

import (
	"net/url"
	"strconv"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/NateScarlet/pixiv/pkg/user"
	"github.com/tidwall/gjson"
)

// SearchResult holds search data and provide useful methods.
type SearchResult struct {
	JSON gjson.Result
}

// ForEach iterates through novel data items.
func (r SearchResult) ForEach(iterator func(key, value gjson.Result) bool) {
	r.JSON.Get("novel.data").ForEach(iterator)

}

// Novels appeared in the search result.
func (r SearchResult) Novels() []Novel {
	ret := make([]Novel, 0, int(r.JSON.Get("#").Int()))
	r.ForEach(func(key, value gjson.Result) bool {
		n := Novel{
			ID:          value.Get("id").String(),
			Title:       value.Get("title").String(),
			Description: value.Get("Description").String(),
			Author: user.User{
				ID:   value.Get("userId").String(),
				Name: value.Get("userName").String(),
			},
			TextCount:     value.Get("textCount").Int(),
			BookmarkCount: value.Get("bookmarkCount").Int(),
			Series: Series{
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

		ret = append(ret, n)
		return true
	})
	return ret

}

// SearchWithClient do request with given client.
func SearchWithClient(c client.Client, query string, page int) (result SearchResult, err error) {
	q := url.Values{}
	if page != 1 {
		q.Set("p", strconv.Itoa(page))
	}

	resp, err := c.Get(c.EndpointURL("/ajax/search/novels/"+query, &q).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := client.ParseAPIResult(resp.Body)
	if err != nil {
		return
	}
	result = SearchResult{JSON: body}
	return
}

// Search calls pixiv novel search api.
func Search(query string, page int) (result SearchResult, err error) {
	return SearchWithClient(*client.Default, query, page)
}
