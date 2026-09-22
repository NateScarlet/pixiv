package client

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
)

// IsLoggedIn checks login status base on `HEAD <server url>/setting_user.php`
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

// setPHPSESSID 让客户端带上 PHPSESSID Cookie 以跳过登录。
//
// 服务地址已在 [New] 装配期校验，这里解析失败属于装配内部错误，直接快速失败。
func (c *Client) setPHPSESSID(v string) {
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
