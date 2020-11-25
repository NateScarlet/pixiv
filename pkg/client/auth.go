package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"github.com/PuerkitoBio/goquery"
)

// IsLoggedIn checks login status base on `HEAD https://www.pixiv.net/setting_user.php`
// response status.
func (c Client) IsLoggedIn() (ret bool, err error) {
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := c.Head(c.EndpointURL("/setting_user.php", nil).String())
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		return false, err
	} else if resp.StatusCode == http.StatusOK {
		return true, err
	}
	return false, errors.New("pixiv: client: unexpected response for login test")
}

func (c *Client) ensureJar() {
	if c.Jar == nil {
		c.Jar, _ = cookiejar.New(nil)
	}
}

// Login with username and password
func (c *Client) Login(username string, password string) (err error) {
	c.ensureJar()

	// Get post key
	resp, err := c.Get("https://accounts.pixiv.net/login?lang=zh")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return
	}
	s := doc.Find(`input[name="post_key"]`)
	if len(s.Nodes) == 0 {
		err = errors.New("pixiv: client: can not found element for post key")
		return
	}
	postKey, ok := s.Attr("value")
	if !ok {
		err = errors.New("pixiv: client: can not extract post key")
		return
	}

	// post
	resp, err = c.PostForm("https://accounts.pixiv.net/api/login?lang=zh",
		url.Values{
			"pixiv_id": []string{username},
			"password": []string{password},
			"post_key": []string{postKey},
		})
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := ParseAPIResult(resp.Body)
	if err != nil {
		return
	}
	if !body.Get("success").Exists() {
		err = fmt.Errorf("pixiv: client: login failed: %+v", body.String())
	}
	return
}

// SetPHPSESSID set client cookie to skip login.
func (c *Client) SetPHPSESSID(v string) {
	c.ensureJar()

	c.Jar.SetCookies(
		c.EndpointURL("", nil),
		[]*http.Cookie{{
			Domain: ".pixiv.net",
			Path:   "/",
			Name:   "PHPSESSID",
			Value:  v,
		}},
	)
}
