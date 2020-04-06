package user

import (
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

	isFetched bool
}

// FetchWithClient do fetch with given client.
func (i *User) FetchWithClient(c client.Client) (err error) {
	if i.isFetched {
		return
	}
	if i.ID == "" {
		return errors.New("no user id specified")
	}
	resp, err := c.Get(c.EndpointURL("/ajax/user/"+i.ID, nil).String())
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
	i.isFetched = true
	return
}

// Fetch additional data from pixiv single user api,
// only fetch once for same struct.
func (i *User) Fetch() (err error) {
	return i.FetchWithClient(*client.Default)
}

// URLWithClient to view web page.
func (i User) URLWithClient(c client.Client) *url.URL {
	return c.EndpointURL("/users/"+i.ID, nil)
}

// URL to view web page.
func (i User) URL() *url.URL {
	return i.URLWithClient(*client.Default)
}
