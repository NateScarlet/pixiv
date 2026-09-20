package client

import (
	"os"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginFromPHPSESSID(t *testing.T) {
	if os.Getenv("PIXIV_PHPSESSID") == "" {
		t.Skip()
		return
	}
	var c = New(WithPHPSESSID(os.Getenv("PIXIV_PHPSESSID")))
	v, err := c.IsLoggedIn()
	require.NoError(t, err)
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
	c := New()
	err := c.Login(username, password)
	require.NoError(t, err)
	v, err := c.IsLoggedIn()
	require.NoError(t, err)
	assert.True(t, v)
}

func TestIsLoggedIn(t *testing.T) {
	testenv.RequireLive(t)
	v, err := New().IsLoggedIn()
	require.NoError(t, err)
	assert.False(t, v)
}
