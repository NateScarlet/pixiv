package artwork

import (
	"context"
	"encoding/json"
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

func TestItemInFetchRankPayloadMaxWidth1200URL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "master thumbnail normalized to regular",
			url:  "https://i.pximg.net/c/480x960/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
			want: "https://i.pximg.net/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
		},
		{
			name: "unrecognized url returned as-is",
			url:  "https://s.pximg.net/common/images/limit_unviewable_s.png",
			want: "https://s.pximg.net/common/images/limit_unviewable_s.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := ItemInFetchRankPayload{json.RawMessage(`{"url":"` + tt.url + `"}`)}
			assert.Equal(t, tt.want, item.MaxWidth1200URL())
		})
	}
}
