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
			"Chapter 1\n[pixivimage:22238487]\np1\n[newpage]\nChapter 2\n[pixivimage:52200823-1]",
		)
		require.NoError(t, err)
		snapshot.Match(t, result, snapshot.OptionExt(".html"))
	})

}
