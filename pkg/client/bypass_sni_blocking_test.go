package client

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByPassSNIBlocking(t *testing.T) {
	var c = new(Client)
	c.BypassSNIBlocking()
	resp, err := c.Get("https://www.pixiv.net")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestByPassSNIBlocking_i_pximg_net(t *testing.T) {
	var c = new(Client)
	c.BypassSNIBlocking()
	var req, err = http.NewRequest(http.MethodGet, "https://i.pximg.net/novel-cover-original/img/2021/01/10/22/47/21/tei14736_2b060b6d13271530d5439f9dbdfe81af.png", nil)
	require.NoError(t, err)
	req.Header.Set("Referer", "https://www.pixiv.net")
	resp, err := c.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
