// Package testenv 提供测试环境门禁，供本模块各包的测试使用。
package testenv

import (
	"os"
	"testing"
)

// RequireLive 跳过依赖真实网络的测试：仅在置 PIXIV_LIVE=1 时运行。
//
// 本包的取回与搜索测试默认只依赖 httptest 本地端点；
// 断言真实 pixiv 行为的用例调用此项加门禁，使测试结果不受运行机器的网络环境影响。
func RequireLive(t testing.TB) {
	t.Helper()
	if os.Getenv("PIXIV_LIVE") == "" {
		t.Skip("需要真实网络，置 PIXIV_LIVE=1 启用")
	}
}
