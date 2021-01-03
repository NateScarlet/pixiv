package client

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoginFromPHPSESSID(t *testing.T) {
	if os.Getenv("PIXIV_PHPSESSID") == "" {
		t.Skip()
		return
	}
	var c = new(Client)
	c.SetPHPSESSID(os.Getenv("PIXIV_PHPSESSID"))
	c.SetDefaultHeader("User-Agent", DefaultUserAgent)
	v, err := c.IsLoggedIn()
	assert.NoError(t, err)
	assert.True(t, v)
}

func TestLogin(t *testing.T) {
	t.Skip("may trigger reCAPTCHA")
	username := os.Getenv("PIXIV_USER")
	password := os.Getenv("PIXIV_PASSWORD")
	if username == "" || password == "" {
		t.Skip("need credentials")
		return
	}
	c := Client{}
	err := c.Login(username, password)
	assert.NoError(t, err)
	v, err := c.IsLoggedIn()
	assert.NoError(t, err)
	assert.True(t, v)
}

func TestIsLoggedIn(t *testing.T) {
	v, err := Client{}.IsLoggedIn()
	assert.NoError(t, err)
	assert.False(t, v)
}
