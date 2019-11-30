package client

import (
	"errors"
	"time"

	"github.com/tidwall/gjson"
)

// ArtworkPages data
type ArtworkPages struct {
	URLs   ImageURLs
	Width  int64
	Height int64
}

// Artwork data
type Artwork struct {
	ID          string
	Title       string
	Type        string
	Description string
	URLs        ImageURLs
	Created     time.Time
	Tags        []string
	Author      User

	Width  int64
	Height int64

	PageCount     int64
	CommentCount  int64
	LikeCount     int64
	ViewCount     int64
	BookmarkCount int64

	Pages []ArtworkPages

	isPagesFetched bool
	isFetched      bool
}

// Fetch additional data from pixiv single artwork api,
// only fetch once for same struct.
func (i *Artwork) Fetch() (err error) {
	if i.isFetched {
		return
	}
	if i.ID == "" {
		return errors.New("no illust id specified")
	}
	resp, err := httpGetBytes(APIIllustURL(i.ID).String())
	if err != nil {
		return
	}
	payload := gjson.ParseBytes(resp)
	err = validateAPIPayload(payload)
	if err != nil {
		return
	}
	data := payload.Get("body")
	i.Title = data.Get("illustTitle").String()
	i.Type = data.Get("illustType").String()
	i.Description = data.Get("description").String()
	i.URLs.Mini = data.Get("urls.mini").String()
	i.URLs.Thumb = data.Get("urls.thumb").String()
	i.URLs.Small = data.Get("urls.small").String()
	i.URLs.Regular = data.Get("urls.regular").String()
	i.URLs.Original = data.Get("urls.original").String()
	i.Created = data.Get("createDate").Time()
	i.Author.ID = data.Get("userId").String()
	i.Author.Name = data.Get("userName").String()
	i.Width = data.Get("width").Int()
	i.Height = data.Get("height").Int()
	i.PageCount = data.Get("pageCount").Int()
	i.CommentCount = data.Get("commentCount").Int()
	i.LikeCount = data.Get("likeCount").Int()
	i.ViewCount = data.Get("viewCount").Int()
	i.BookmarkCount = data.Get("bookmarkCount").Int()
	tags := []string{}
	for _, i := range data.Get("tags.tags.#.tag").Array() {
		tags = append(tags, i.String())
	}
	i.Tags = tags
	i.isFetched = true
	return
}

// FetchPages get all pages for artwork from pixiv artwork pages api,
// only fetch once for same struct.
func (i *Artwork) FetchPages() (err error) {
	if i.isPagesFetched {
		return
	}
	if i.ID == "" {
		return errors.New("no illust id specified")
	}
	resp, err := httpGetBytes(APIIllustPagesURL(i.ID).String())
	if err != nil {
		return
	}
	payload := gjson.ParseBytes(resp)
	err = validateAPIPayload(payload)
	if err != nil {
		return
	}
	pages := []ArtworkPages{}
	payload.Get("body").ForEach(func(key, value gjson.Result) bool {
		i := ArtworkPages{}
		i.URLs.Mini = value.Get("urls.thumb_mini").String()
		i.URLs.Thumb = i.URLs.Mini
		i.URLs.Small = value.Get("urls.small").String()
		i.URLs.Regular = value.Get("urls.regular").String()
		i.URLs.Original = value.Get("urls.original").String()
		i.Width = value.Get("width").Int()
		i.Height = value.Get("height").Int()
		pages = append(pages, i)
		return true
	})
	i.Pages = pages
	i.isPagesFetched = true
	return
}
