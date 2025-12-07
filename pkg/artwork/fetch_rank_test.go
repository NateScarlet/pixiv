package artwork

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRank(t *testing.T) {
	payload, err := FetchRank(context.Background(), DailyRank)
	require.NoError(t, err)
	var n int
	for item := range payload.Items() {
		assert.NotEmpty(t, item.ID())
		assert.NotEmpty(t, item.Title())
		assert.NotEmpty(t, item.AuthorID())
		assert.NotEmpty(t, item.AuthorName())
		assert.NotEmpty(t, item.Width())
		assert.NotEmpty(t, item.Height())
		n++
	}
	assert.GreaterOrEqual(t, n, 45)
}
