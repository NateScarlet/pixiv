package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchUser(t *testing.T) {
	var ctx = context.Background()
	i := User{ID: "789096"}
	err := i.Fetch(ctx)
	require.NoError(t, err)
	t.Log(i)
	assert.Equal(t, "789096", i.ID)
	assert.Equal(t, "CHN^NateScarlet", i.Name)
	assert.NotEmpty(t, i.Avatar.Mini)
	assert.NotEmpty(t, i.Avatar.Thumb)
	assert.Equal(t, "https://www.pixiv.net/users/789096", i.URL(ctx).String())
}
