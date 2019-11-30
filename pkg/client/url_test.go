package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArtworkSearchAPIURL(t *testing.T) {
	assert.Equal(t, "https://www.pixiv.net/ajax/search/artworks/sometag", APIArtworkSearchURL("sometag", 1).String())
	assert.Equal(t, "https://www.pixiv.net/ajax/search/artworks/sometag?p=2", APIArtworkSearchURL("sometag", 2).String())
}
func TestIllustAPIURL(t *testing.T) {
	assert.Equal(t, "https://www.pixiv.net/ajax/illust/123456", APIIllustURL("123456").String())
}
