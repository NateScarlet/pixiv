package client

import (
	"os"
	"testing"

	"github.com/joho/godotenv"

	"github.com/stretchr/testify/assert"
)

func getCredential(t *testing.T) (username string, password string) {
	godotenv.Load("../../.env", ".env")
	username = os.Getenv("PIXIV_USER")
	password = os.Getenv("PIXIV_PASSWORD")
	if username == "" || password == "" {
		t.Skip()
	}

	return
}

func TestLoginFromCookie(t *testing.T) {
	if os.Getenv("PIXIV_PHPSESSID") == "" {
		t.Skip()
		return
	}
	loggedIn, err := IsLoggedIn()
	assert.NoError(t, err)
	assert.False(t, loggedIn)
	err = LoginFromEnv()
	assert.NoError(t, err)
	loggedIn, err = IsLoggedIn()
	assert.NoError(t, err)
	assert.True(t, loggedIn)
}

func TestLogin(t *testing.T) {
	t.Skip("May require reCAPTCHA")
	username, password := getCredential(t)
	result, err := IsLoggedIn()
	assert.NoError(t, err)
	assert.False(t, result)
	err = Login(username, password)
	assert.NoError(t, err)
	result, err = IsLoggedIn()
	assert.NoError(t, err)
	assert.True(t, result)
}

func TestIsLoggedIn(t *testing.T) {
	result, err := IsLoggedIn()
	assert.NoError(t, err)
	assert.False(t, result)
	getCredential(t)
	err = LoginFromEnv()
	assert.NoError(t, err)
	result, err = IsLoggedIn()
	assert.NoError(t, err)
	assert.True(t, result)
}
