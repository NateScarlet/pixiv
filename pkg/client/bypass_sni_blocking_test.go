package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestByPassSNIBlocking(t *testing.T) {
	var c = new(Client)
	c.BypassSNIBlocking()
	resp, err := c.Get("https://www.pixiv.net")
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
