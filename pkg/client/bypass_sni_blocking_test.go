package client

import (
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
