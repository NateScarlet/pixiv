package image

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromURL(t *testing.T) {
	// 实测返还的排行榜缩略图: /c/480x960/img-master/img/..._master1200.jpg
	sample := "https://i.pximg.net/c/480x960/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg"

	tests := []struct {
		name string
		in   string
		want URLs
	}{
		{
			name: "c-prefixed master1200 thumbnail",
			in:   sample,
			want: URLs{
				Mini:     "https://i.pximg.net/c/48x48/img-master/img/2026/08/26/00/00/29/148882180_p0_square1200.jpg",
				Thumb:    "https://i.pximg.net/c/250x250_80_a2/img-master/img/2026/08/26/00/00/29/148882180_p0_square1200.jpg",
				Small:    "https://i.pximg.net/c/540x540_70/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
				Regular:  "https://i.pximg.net/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
				Original: "https://i.pximg.net/img-original/img/2026/08/26/00/00/29/148882180_p0.jpg",
			},
		},
		{
			name: "plain regular url",
			in:   "https://i.pximg.net/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
			want: URLs{
				Mini:     "https://i.pximg.net/c/48x48/img-master/img/2026/08/26/00/00/29/148882180_p0_square1200.jpg",
				Thumb:    "https://i.pximg.net/c/250x250_80_a2/img-master/img/2026/08/26/00/00/29/148882180_p0_square1200.jpg",
				Small:    "https://i.pximg.net/c/540x540_70/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
				Regular:  "https://i.pximg.net/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
				Original: "https://i.pximg.net/img-original/img/2026/08/26/00/00/29/148882180_p0.jpg",
			},
		},
		{
			name: "custom-thumb square1200 thumbnail",
			in:   "https://i.pximg.net/c/250x250_80_a2/custom-thumb/img/2026/08/26/00/00/29/148882180_p0_custom1200.jpg",
			want: URLs{
				Mini:     "https://i.pximg.net/c/48x48/img-master/img/2026/08/26/00/00/29/148882180_p0_square1200.jpg",
				Thumb:    "https://i.pximg.net/c/250x250_80_a2/img-master/img/2026/08/26/00/00/29/148882180_p0_square1200.jpg",
				Small:    "https://i.pximg.net/c/540x540_70/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
				Regular:  "https://i.pximg.net/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg",
				Original: "https://i.pximg.net/img-original/img/2026/08/26/00/00/29/148882180_p0.jpg",
			},
		},
		{
			name: "original url",
			in:   "https://i.pximg.net/img-original/img/2026/08/26/00/00/29/148882180_p0.png",
			want: URLs{
				Mini:     "https://i.pximg.net/c/48x48/img-master/img/2026/08/26/00/00/29/148882180_p0_square1200.png",
				Thumb:    "https://i.pximg.net/c/250x250_80_a2/img-master/img/2026/08/26/00/00/29/148882180_p0_square1200.png",
				Small:    "https://i.pximg.net/c/540x540_70/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.png",
				Regular:  "https://i.pximg.net/img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.png",
				Original: "https://i.pximg.net/img-original/img/2026/08/26/00/00/29/148882180_p0.png",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromURL(tt.in)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFromURLUnrecognized(t *testing.T) {
	tests := []string{
		// 非 pixiv 图片(如不可查看占位图)
		"https://s.pximg.net/common/images/limit_unviewable_s.png",
		// 缺少主机名
		"/c/48x48/img-master/img/2026/08/26/00/00/29/1_p0_master1200.jpg",
		// 非法 URL
		"://bad",
	}
	for _, in := range tests {
		in := in
		t.Run(in, func(t *testing.T) {
			_, err := FromURL(in)
			require.Error(t, err)
			require.Contains(t, err.Error(), in)
		})
	}
}

func TestFromURLKeepsHostAndScheme(t *testing.T) {
	got, err := FromURL("http://i.pximg.net/c/48x48/img-master/img/2026/08/26/00/00/29/1_p0_square1200.jpg")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got.Regular, "http://i.pximg.net/"))
}