package novel

import (
	"errors"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/NateScarlet/pixiv/pkg/user"
)

// Series data
type Series struct {
	ID    string
	Title string
}

// Novel data
type Novel struct {
	ID          string
	Title       string
	Description string
	CoverURL    string
	Content     string
	Created     time.Time
	Uploaded    time.Time
	Author      user.User
	Series      Series
	Tags        []string

	TextCount     int64
	PageCount     int64
	CommentCount  int64
	LikeCount     int64
	ViewCount     int64
	BookmarkCount int64

	isFetched bool
}

// FetchWithClient do request with given client.
func (i *Novel) FetchWithClient(c client.Client) (err error) {
	if i.isFetched {
		return
	}
	if i.ID == "" {
		return errors.New("no novel id specified")
	}
	resp, err := c.Get(c.EndpointURL("/ajax/novel/"+i.ID, nil).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, err := client.ParseAPIResult(resp.Body)
	if err != nil {
		return
	}
	i.Title = data.Get("title").String()
	i.Description = data.Get("description").String()
	i.CoverURL = data.Get("coverUrl").String()
	i.Created = data.Get("createDate").Time()
	i.Uploaded = data.Get("uploadDate").Time()
	i.Author.ID = data.Get("userId").String()
	i.Author.Name = data.Get("userName").String()
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
	i.Content = data.Get("content").String()

	i.isFetched = true
	return
}

// Fetch additional data from pixiv single novel api (require login),
// only fetch once for same struct.
func (i *Novel) Fetch() (err error) {
	return i.FetchWithClient(*client.Default)
}
