package client

import (
	"errors"

	"github.com/tidwall/gjson"
)

// User data.
type User struct {
	ID         string
	Name       string
	AvatarURLs ImageURLs

	isFetched bool
}

// Fetch additional data from pixiv single user api,
// only fetch once for same struct.
func (i *User) Fetch() (err error) {
	if i.isFetched {
		return
	}
	if i.ID == "" {
		return errors.New("no user id specified")
	}
	resp, err := httpGetBytes(APIUserURL(i.ID).String())
	if err != nil {
		return
	}
	payload := gjson.ParseBytes(resp)
	err = validateAPIPayload(payload)
	if err != nil {
		return
	}
	data := payload.Get("body")
	i.Name = data.Get("name").String()
	i.AvatarURLs.Mini = data.Get("image").String()
	i.AvatarURLs.Thumb = data.Get("imageBig").String()
	i.isFetched = true
	return
}
