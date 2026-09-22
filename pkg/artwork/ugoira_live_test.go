package artwork

import (
	"context"
	"slices"
	"testing"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ugoiraLiveID 是真实动图作品（illustType=2），2026-09-22 实测其
// ugoira_meta 匿名接口可解析出 zip 地址与 21 帧时序。
const ugoiraLiveID = "44332434"

// TestFetchUgoiraMetaLive 验证真实匿名接口解析动图元数据：
// 需要网络可用；默认解析端点不可达时置 PIXIV_DNS_QUERY_URL=dns://127.0.0.1。
func TestFetchUgoiraMetaLive(t *testing.T) {
	testenv.RequireLive(t)
	m, err := FetchUgoiraMeta(context.Background(), ugoiraLiveID)
	require.NoError(t, err)
	assert.Equal(t, ugoiraLiveID, m.ID())
	require.NotEmpty(t, m.ZipURL(), "压缩版 zip 地址不应为空")
	require.NotEmpty(t, m.OriginalZipURL(), "原图 zip 地址不应为空")
	require.NotEmpty(t, m.MimeType())
	require.NotEmpty(t, slices.Collect(m.Frames()), "帧时序不应为空")
	assert.NotEmpty(t, m.Raw())
	assert.NotNil(t, m.Response())
}
