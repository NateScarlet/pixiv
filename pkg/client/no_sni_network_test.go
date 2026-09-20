package client

import (
	"net/http"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// i.pximg.net 取图依赖「不发送 SNI」，经路由传输自动生效。
func TestNoSNITransportFetchesIPximgNet(t *testing.T) {
	testenv.RequireLive(t)
	var c = New()
	var req, err = http.NewRequest(http.MethodGet,
		"https://i.pximg.net/novel-cover-original/img/2021/01/10/22/47/21/tei14736_2b060b6d13271530d5439f9dbdfe81af.png", nil)
	require.NoError(t, err)
	req.Header.Set("Referer", "https://www.pixiv.net")
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
}
