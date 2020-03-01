package client

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/tidwall/gjson"
)

type ArtworkRankItem struct {
	Artwork
	Rank         int
	PreviousRank int
	JSON         gjson.Result
}

type ArtworkRank struct {
	/* required, possible rank modes:
		- daily
	    - weekly
	    - monthly
	    - rookie
	    - original
	    - male
	    - female
	    - daily_r18
	    - weekly_r18
	    - male_r18
	    - female_r18
	    - r18g
	*/
	Mode string
	/* optional, possible rank content:
	    - all (default)
	    - illust
		- ugoira
		- manga
	*/
	Content string
	Date    time.Time
	Page    int
	Items   []ArtworkRankItem
}

func (rank ArtworkRank) URL(query ...string) *url.URL {
	u := SiteURL("ranking.php")
	q := u.Query()
	q.Set("mode", rank.Mode)
	if rank.Content != "" {
		q.Set("content", rank.Content)
	}
	if !rank.Date.IsZero() {
		q.Set("date", rank.Date.Format("20060102"))
	}
	if rank.Page > 1 {
		q.Set("p", strconv.Itoa(rank.Page))
	}
	for index, _ := range query {
		if index%2 == 1 {
			q.Set(query[index-1], query[index])
		}
	}
	u.RawQuery = q.Encode()
	return u
}

func (rank *ArtworkRank) Fetch() (err error) {
	payload, err := httpGetJSON(rank.URL("format", "json").String())
	if err != nil {
		return
	}

	contents := payload.Get("contents").Array()
	items := make([]ArtworkRankItem, len(contents))
	for index, i := range contents {
		items[index] = ArtworkRankItem{
			JSON: i,
			Artwork: Artwork{
				ID:      i.Get("illust_id").String(),
				Title:   i.Get("title").String(),
				Type:    i.Get("illust_type").String(),
				Width:   i.Get("width").Int(),
				Height:  i.Get("height").Int(),
				Created: time.Unix(i.Get("illust_upload_timestamp").Int(), 0),
				URLs: ImageURLs{
					Regular: i.Get("url").String(),
				},
				Author: User{
					ID:   i.Get("user_id").String(),
					Name: i.Get("user_name").String(),
					AvatarURLs: ImageURLs{
						Mini: i.Get("profile_img").String(),
					},
				},
				PageCount: i.Get("illust_page_count").Int(),
			},
			Rank:         int(i.Get("rank").Int()),
			PreviousRank: int(i.Get("yes_rank").Int()),
		}
	}

	if len(items) == 0 {
		return fmt.Errorf("No rank items found: url=%s", rank.URL().String())
	}
	rank.Items = items
	return
}
