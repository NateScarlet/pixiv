package dns

import (
	"context"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDOHResolver(t *testing.T) {
	testenv.RequireLive(t)
	// 用默认端点验证解析机制本身即可；其他第三方端点的可用性属于部署环境，
	// 不是本库的代码行为（可用服务清单见 docs/env-vars.rst）。
	r := NewDOHResolver("https://1.1.1.1/dns-query")
	ip, err := r.Resolve(context.Background(), "www.pixiv.net")
	require.NoError(t, err)
	assert.NotEmpty(t, ip)
	assert.Regexp(t, `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`, ip[0].String())
}
