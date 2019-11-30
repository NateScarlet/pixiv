package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"

	"golang.org/x/net/publicsuffix"
)

var cookieJar, _ = cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})

var client = &http.Client{Jar: cookieJar}

// HTTPClient for custom request with session.
var HTTPClient = client
var loginMu = &sync.Mutex{}

// LoginFromEnv login when account configured and not logged in
func LoginFromEnv() error {
	loginMu.Lock()
	defer loginMu.Unlock()

	// Use `PHPSESSID`
	phpsessid := os.Getenv("PIXIV_PHPSESSID")
	if phpsessid != "" {
		client.Jar.SetCookies(SiteURL(""),
			[]*http.Cookie{&http.Cookie{
				Domain: ".pixiv.net",
				Path:   "/",
				Name:   "PHPSESSID",
				Value:  phpsessid,
			}})
	}

	// Use username and password.
	username := os.Getenv("PIXIV_USER")
	password := os.Getenv("PIXIV_PASSWORD")

	if username == "" || password == "" {
		return nil
	}
	loggedIn, err := IsLoggedIn()
	if err != nil {
		return err
	}
	if loggedIn {
		return nil
	}
	return Login(username, password)
}

// IsLoggedIn checks login status base on `HEAD https://www.pixiv.net/setting_user.php`
// response status.
func IsLoggedIn() (ret bool, err error) {
	c := &http.Client{
		Jar: client.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := c.Head(SiteURL("/setting_user.php").String())
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		return false, err
	} else if resp.StatusCode == http.StatusOK {
		return true, err
	}
	return false, errors.New("unexpected response for login test")
}

// Login pixiv
func Login(username string, password string) (err error) {
	doc, err := httpGetDocument("https://accounts.pixiv.net/login?lang=zh")
	if err != nil {
		return
	}
	s := doc.Find(`input[name="post_key"]`)
	if len(s.Nodes) == 0 {
		err = errors.New("Can not found element for post key")
		return
	}
	postKey, ok := s.Attr("value")
	if !ok {
		err = errors.New("Can not extract post key")
		return
	}

	resp, err := client.PostForm("https://accounts.pixiv.net/api/login?lang=zh",
		url.Values{
			"pixiv_id": []string{username},
			"password": []string{password},
			"post_key": []string{postKey},
		})
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var result struct {
		Body    map[string]interface{} `json:"body"`
		Error   bool                   `json:"error"`
		Message string                 `json:"message"`
	}
	json.Unmarshal(body, &result)
	if result.Body["success"] == nil {
		err = fmt.Errorf("Login failed: %+v", result)
		return
	}

	return
}
