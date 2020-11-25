package artwork

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/NateScarlet/pixiv/pkg/image"
	"github.com/NateScarlet/pixiv/pkg/user"
	"github.com/tidwall/gjson"
)

// Page is a artwork page.
type Page struct {
	Image  image.URLs
	Width  int64
	Height int64
}

// Artwork data
type Artwork struct {
	ID          string
	Title       string
	Type        string
	Description string
	Image       image.URLs
	Created     time.Time
	Tags        []string
	Author      user.User

	Width  int64
	Height int64

	PageCount     int64
	CommentCount  int64
	LikeCount     int64
	ViewCount     int64
	BookmarkCount int64

	Pages []Page
}

// Fetch additional data from pixiv single artwork api,
func (i *Artwork) Fetch(ctx context.Context) (err error) {
	if i.ID == "" {
		return errors.New("pixiv: artwork: no id specified")
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/illust/"+i.ID, nil).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := client.ParseAPIResult(resp.Body)
	if err != nil {
		return
	}
	i.Title = body.Get("illustTitle").String()
	i.Type = body.Get("illustType").String()
	i.Description = body.Get("description").String()
	i.Image.Mini = body.Get("urls.mini").String()
	i.Image.Thumb = body.Get("urls.thumb").String()
	i.Image.Small = body.Get("urls.small").String()
	i.Image.Regular = body.Get("urls.regular").String()
	i.Image.Original = body.Get("urls.original").String()
	i.Created = body.Get("createDate").Time()
	i.Author.ID = body.Get("userId").String()
	i.Author.Name = body.Get("userName").String()
	i.Width = body.Get("width").Int()
	i.Height = body.Get("height").Int()
	i.PageCount = body.Get("pageCount").Int()
	i.CommentCount = body.Get("commentCount").Int()
	i.LikeCount = body.Get("likeCount").Int()
	i.ViewCount = body.Get("viewCount").Int()
	i.BookmarkCount = body.Get("bookmarkCount").Int()
	tags := []string{}
	for _, i := range body.Get("tags.tags.#.tag").Array() {
		tags = append(tags, i.String())
	}
	i.Tags = tags
	return
}

// FetchPages get all pages for artwork from pixiv artwork pages api,
func (i *Artwork) FetchPages(ctx context.Context) (err error) {
	if i.ID == "" {
		return errors.New("pixiv: artwork: no id specified")
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/illust/"+i.ID+"/pages", nil).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, err := client.ParseAPIResult(resp.Body)
	if err != nil {
		return
	}
	pages := make([]Page, 0, int(data.Get("#").Int()))
	data.ForEach(func(key, value gjson.Result) bool {
		i := Page{}
		i.Image.Mini = value.Get("urls.thumb_mini").String()
		i.Image.Thumb = i.Image.Mini
		i.Image.Small = value.Get("urls.small").String()
		i.Image.Regular = value.Get("urls.regular").String()
		i.Image.Original = value.Get("urls.original").String()
		i.Width = value.Get("width").Int()
		i.Height = value.Get("height").Int()
		pages = append(pages, i)
		return true
	})
	i.Pages = pages
	return
}

// URL to view artwork web page.
func (i Artwork) URL(ctx context.Context) *url.URL {
	return client.For(ctx).EndpointURL("/artworks/"+i.ID, nil)
}
