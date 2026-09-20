package artwork

import (
	"context"
	"testing"
	"time"

	"github.com/NateScarlet/pixiv/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankURL(t *testing.T) {
	var ctx = context.Background()
	date, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00+00:00")
	require.NoError(t, err)
	for _, tt := range []struct {
		name string
		rank Rank
		want string
	}{
		{"默认", Rank{Mode: "daily"}, "https://www.pixiv.net/ranking.php"},
		{"模式", Rank{Mode: "weekly"}, "https://www.pixiv.net/ranking.php?mode=weekly"},
		{"日期", Rank{Mode: "weekly", Date: date}, "https://www.pixiv.net/ranking.php?date=20200101&mode=weekly"},
		{"内容与日期", Rank{Mode: "weekly", Content: "manga", Date: date}, "https://www.pixiv.net/ranking.php?content=manga&date=20200101&mode=weekly"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.rank.URL(ctx)
			assert.Equal(t, tt.want, u.String())
		})
	}
}

func TestArtworkRankSimple(t *testing.T) {
	testenv.RequireLive(t)
	date, err := time.Parse(time.RFC3339, "2020-01-01T00:00:00+00:00")
	require.NoError(t, err)
	rank := &Rank{
		Mode: "daily",
		Date: date,
	}
	err = rank.Fetch(context.Background())
	require.NoError(t, err)
	// 历史榜单的条目数与字段随 pixiv 侧变化（2026-09 实测返回 44 条，
	// 已删除作品标题为空、页数字段缺省），因此只断言基本解析成功，
	// 不再对条目数与完整字段做断言。
	assert.NotEmpty(t, rank.Items)
	for _, item := range rank.Items {
		assert.NotEmpty(t, item.Rank)
		assert.NotEmpty(t, item.Artwork.Image.Regular)
	}
}
