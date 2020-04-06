package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFetchUser(t *testing.T) {
	i := User{ID: "789096"}
	err := i.Fetch()
	t.Log(i)
	assert.NoError(t, err)
	assert.Equal(t, "789096", i.ID)
	assert.Equal(t, "CHN^NateScarlet", i.Name)
	assert.NotEmpty(t, i.Avatar.Mini)
	assert.NotEmpty(t, i.Avatar.Thumb)
}
