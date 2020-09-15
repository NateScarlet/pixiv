package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveHostname(t *testing.T) {

	ip, err := resolveHostname("www.pixiv.net")
	assert.NoError(t, err)
	assert.Regexp(t, `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`, ip)
}
