package novel

import (
	"context"
	"testing"

	"github.com/NateScarlet/snapshot/pkg/snapshot"
	"github.com/stretchr/testify/require"
)

func TestHTMLContent(t *testing.T) {
	var ctx = context.Background()
	t.Run("simple", func(t *testing.T) {
		result, err := HTMLContent(
			ctx,
			SimpleContentRenderer{},
			`[chapter:[[rb:本章标题 > 假名]]]
[jump:2]
[[jumpuri:标题 > http://example.com]]
[newpage]
[pixivimage:22238487]
[newpage]
[pixivimage:52200823-1]
p1

p2


p3
`,
		)
		require.NoError(t, err)
		snapshot.Match(t, result, snapshot.OptionExt(".html"))
	})

	t.Run("real", func(t *testing.T) {
		i := Novel{ID: "14443124"}
		err := i.Fetch(ctx)
		require.NoError(t, err)
		result, err := i.HTMLContent(ctx, nil)
		require.NoError(t, err)
		snapshot.Match(t, result, snapshot.OptionExt(".html"))
	})
}
