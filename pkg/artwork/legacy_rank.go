package artwork

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"strconv"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/tidwall/gjson"
)

// RankItem contains artwork and rank info.
type RankItem struct {
	Artwork
	Rank         int
	PreviousRank int
	JSON         gjson.Result
}

// Rank contains data for one rank page.
type Rank struct {
	/* required, possible rank modes:
		- daily (default)
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
	Items   []RankItem
}

// URLWithQuery to rank page.
func (rank Rank) URLWithQuery(ctx context.Context, q *url.Values) *url.URL {
	if q == nil {
		q = &url.Values{}
	}
	if rank.Mode != "" && rank.Mode != "daily" {
		q.Set("mode", rank.Mode)
	}
	if rank.Content != "" {
		q.Set("content", rank.Content)
	}
	if !rank.Date.IsZero() {
		q.Set("date", rank.Date.Format("20060102"))
	}
	if rank.Page > 1 {
		q.Set("p", strconv.Itoa(rank.Page))
	}
	return client.For(ctx).EndpointURL("/ranking.php", q)
}

// URL to rank page.
func (rank Rank) URL(ctx context.Context) *url.URL {
	return rank.URLWithQuery(ctx, nil)
}

// Fetch rank
func (rank *Rank) Fetch(ctx context.Context) (err error) {
	resp, err := client.For(ctx).GetWithContext(ctx, rank.URLWithQuery(ctx, &url.Values{"format": {"json"}}).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	result := gjson.Parse(string(data)).Get("contents")
	items := make([]RankItem, 0, int(result.Get("#").Int()))
	result.ForEach(
		func(k, v gjson.Result) bool {
			i := RankItem{}
			i.JSON = v
			i.Rank = int(v.Get("rank").Int())
			i.PreviousRank = int(v.Get("yes_rank").Int())
			i.Artwork.ID = v.Get("illust_id").String()
			i.Artwork.Title = v.Get("title").String()
			i.Artwork.Type = v.Get("illust_type").String()
			i.Artwork.Width = v.Get("width").Int()
			i.Artwork.Height = v.Get("height").Int()
			i.Artwork.Created = time.Unix(v.Get("illust_upload_timestamp").Int(), 0)
			i.Image.Regular = v.Get("url").String()
			i.Author.ID = v.Get("user_id").String()
			i.Author.ID = v.Get("user_name").String()
			i.Author.Name = v.Get("user_id").String()
			i.Author.Avatar.Mini = v.Get("profile_img").String()
			i.PageCount = v.Get("illust_page_count").Int()
			items = append(items, i)
			return true
		},
	)
	if len(items) == 0 {
		return fmt.Errorf("pixiv: artwork: no rank items found: url=%s", resp.Request.URL.String())
	}
	rank.Items = items
	return
}
