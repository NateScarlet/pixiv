package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/tidwall/gjson"
)

// Client to send request to pixiv server.
type Client struct {
	ServerURL string
	http.Client
}

type dns struct {
	Answer []struct {
		Name string `json:"name"`
		Data string `json:"data"`
	} `json:"Answer"`
}

func GetRealIp() string {
	res, err := http.Get("https://1.1.1.1/dns-query?ct=application/dns-json&name=pixiv.net&type=A")
	if err != nil {
		println(err.Error())
		return ""
	}
	var paraseDns dns
	data, _ := ioutil.ReadAll(res.Body)
	_ = json.Unmarshal(data, &paraseDns)
	return paraseDns.Answer[0].Data
}

var realip = ""

// EndpointURL returns url for server endpint.
func (c Client) EndpointURL(path string, values *url.Values) *url.URL {
	s := c.ServerURL
	if s == "" {
		if bypass {
			if realip == "" {
				realip = GetRealIp()
			}
			ip := realip
			s = fmt.Sprintf("https://%s", ip)
		} else {
			s = "https://www.pixiv.net"
		}
	}

	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	u.Path = path
	if values != nil {
		u.RawQuery = values.Encode()
	}
	return u
}

// ParseAPIResult parses error from json api response, and returns body part.
func ParseAPIResult(r io.Reader) (ret gjson.Result, err error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}
	s := string(data)
	if !gjson.Valid(s) {
		err = fmt.Errorf("invalid json: %s", s)
		return
	}
	ret = gjson.Parse(s)
	hasError := ret.Get("error").Bool()
	message := ret.Get("message").String()
	ret = ret.Get("body")
	if hasError {
		err = fmt.Errorf("pixiv api error: %s", message)
	}
	return
}

// Default client auto login with PIXIV_PHPSESSID env var.
var Default = NewClient(false)
var bypass bool

func NewClient(isBypass bool) *Client {
	bypass = isBypass
	if isBypass {
		return &Client{
			Client: http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			},
			ServerURL: fmt.Sprintf("https://%s", GetRealIp()),
		}
	} else {
		return &Client{}
	}
}

func SetDefaultToBypass() {
	Default = NewClient(true)
}

func (c *Client) Get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if bypass {
		req.Host = "www.pixiv.net"
	}
	return c.Do(req)
}

func (c *Client) Download(url string, id string) (resp *http.Response, err error) {
	ref := "https://www.pixiv.net/artworks/%s" + id
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Referer", "")
	req.Header.Set("Referer", ref)
	return c.Do(req)
}

func init() {
	Default.SetPHPSESSID(os.Getenv("PIXIV_PHPSESSID"))
}
