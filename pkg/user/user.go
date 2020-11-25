package user

import (
	"context"
	"errors"
	"net/url"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/NateScarlet/pixiv/pkg/image"
)

// User data.
type User struct {
	ID     string
	Name   string
	Avatar image.URLs
}

// Fetch additional data from pixiv single user api,
func (i *User) Fetch(ctx context.Context) (err error) {
	if i.ID == "" {
		return errors.New("pixiv: user: no id specified")
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/user/"+i.ID, nil).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := client.ParseAPIResult(resp.Body)
	if err != nil {
		return
	}
	i.Name = body.Get("name").String()
	i.Avatar.Mini = body.Get("image").String()
	i.Avatar.Thumb = body.Get("imageBig").String()
	return
}

// URL to view web page.
func (i User) URL(ctx context.Context) *url.URL {
	return client.For(ctx).EndpointURL("/users/"+i.ID, nil)
}
