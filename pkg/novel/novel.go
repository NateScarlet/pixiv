package novel

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

// Series data
type Series struct {
	ID    string
	Title string
}

// URLWithClient to view web page.
func (i Series) URLWithClient(c client.Client) *url.URL {
	return c.EndpointURL("/novel/series/"+i.ID, nil)
}

// URL to view web page.
func (i Series) URL() *url.URL {
	return i.URLWithClient(*client.Default)
}

// Novel data
type Novel struct {
	ID             string
	Title          string
	Description    string
	CoverURL       string
	Content        string
	Created        time.Time
	Uploaded       time.Time
	Author         user.User
	Series         Series
	Tags           []string
	EmbeddedImages map[string]image.URLs

	TextCount     int64
	PageCount     int64
	CommentCount  int64
	LikeCount     int64
	ViewCount     int64
	BookmarkCount int64
	CreationMethod CreationMethod
}

// Fetch additional data from pixiv single novel api (require login),
func (i *Novel) Fetch(ctx context.Context) (err error) {
	if i.ID == "" {
		return errors.New("pixiv: novel: no id specified")
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/novel/"+i.ID, nil).String())
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
	// textEmbeddedImages 为 null 时 ForEach 仍会回调一次，产生空 key 的空条目。
	// 因此仅在字段为对象时遍历。
	embeddedImages := data.Get("textEmbeddedImages")
	if embeddedImages.IsObject() {
		embeddedImages.ForEach(func(key, value gjson.Result) bool {
			if i.EmbeddedImages == nil {
				i.EmbeddedImages = make(map[string]image.URLs)
			}
			i.EmbeddedImages[key.String()] = image.URLs{
				Thumb:    value.Get("urls.128x128").String(),
				Small:    value.Get("urls.480mw").String(),
				Regular:  value.Get("urls.1200x1200").String(),
				Original: value.Get("urls.original").String(),
			}
			return true
		})
	}
	i.Content = data.Get("content").String()
	i.CreationMethod = parseCreationMethod(data.Get("aiType"))
	return
}

func parseCreationMethod(v gjson.Result) CreationMethod {
	switch v.Int() {
	case 1:
		return ManuallyCreated
	case 2:
		return AIGenerated
	default: // 包括0和其他未定义值
		return UnknownCreationMethod
	}
}

// URL to view web page.
func (i Novel) URL(ctx context.Context) *url.URL {
	return client.For(ctx).EndpointURL("/novel/show.php", &url.Values{"id": {i.ID}})
}

// HTMLContent from content, provide nil as renderer to use default renderer,
func (i Novel) HTMLContent(ctx context.Context, renderer ContentRenderer) (string, error) {
	if renderer == nil {
		renderer = SimpleContentRenderer{EmbeddedImages: i.EmbeddedImages}
	}
	return HTMLContent(ctx, renderer, i.Content)
}

// CreationMethod indicates the novel's creation method
// CreationMethod 表示小说的创作方式
type CreationMethod int

const (
	UnknownCreationMethod CreationMethod = iota // Unknown creation method | 未知创作类型
	ManuallyCreated                             // Created without AI tools | 非AI生成作品
	AIGenerated                                 // Created with AI technology | AI生成作品
)
