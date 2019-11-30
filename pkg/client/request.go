package client

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

func httpGet(url string) (resp *http.Response, err error) {
	resp, err = client.Get(url)
	if err != nil {
		return
	}
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("Request failed: %s : %s", url, resp.Status)
		return
	}
	return
}

func httpGetBytes(url string) (ret []byte, err error) {
	resp, err := httpGet(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	ret, err = ioutil.ReadAll(resp.Body)
	return
}

func httpGetDocument(url string) (doc *goquery.Document, err error) {
	resp, err := httpGet(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	doc, err = goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return
	}
	return
}

func validateAPIPayload(payload gjson.Result) error {
	if !gjson.Valid(payload.Raw) {
		return fmt.Errorf("Pixiv api error: invalid json: %s", payload.Raw)
	}
	hasError := payload.Get("error").Bool()
	message := payload.Get("message").String()

	if hasError {
		return fmt.Errorf("Pixiv api error: %s", message)
	}
	return nil
}
