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

// TestIsImageURL 断言地址判读按路径段进行：
// 接受库自身会交给调用者的各类地址（含不具备各尺寸结构的小说封面、
// 头像、背景图与动图 zip），拒绝非 pixiv CDN 资源地址。
func TestIsImageURL(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want bool
	}{
		{"regular", "https://i.pximg.net/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg", true},
		{"original", "https://i.pximg.net/img-original/img/2026/09/07/00/00/12/149365161_p0.png", true},
		{"带缩放前缀的缩略图", "https://i.pximg.net/c/48x48/img-master/img/2026/09/07/00/00/12/149365161_p0_square1200.jpg", true},
		{"自定义裁剪缩略图", "https://i.pximg.net/c/250x250_80_a2/custom-thumb/img/2026/09/07/00/00/12/149365161_p0_custom1200.jpg", true},
		{"小说封面原图", "https://i.pximg.net/novel-cover-original/img/2021/01/10/22/47/21/tei14736_2b060b6d13271530d5439f9dbdfe81af.png", true},
		{"带缩放的小说封面", "https://i.pximg.net/c/600x600/novel-cover-master/img/2021/01/10/22/59/38/14443124_77_master1200.jpg", true},
		{"作者头像", "https://i.pximg.net/user-profile/img/2022/09/23/01/34/52/23368434_0daa45f98a51e102a4ef48411bffe087_50.jpg", true},
		{"用户背景图", "https://i.pximg.net/background/img/2021/01/10/22/47/21/abc.jpg", true},
		{"动图压缩版 zip", "https://i.pximg.net/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira600x600.zip", true},
		{"动图原图 zip", "https://i.pximg.net/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira1920x1080.zip", true},
		{"http 亦可", "http://i.pximg.net/c/48x48/img-master/img/2026/08/26/00/00/29/1_p0_square1200.jpg", true},
		{"画作页面", "https://www.pixiv.net/artworks/149365161", false},
		{"API 地址", "https://www.pixiv.net/ajax/illust/149365161", false},
		{"占位图不是可取回的图片", "https://i.pximg.net/common/images/limit_unviewable_s.png", false},
		{"缺主机名", "/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg", false},
		{"非 http 协议", "ftp://i.pximg.net/img-master/img/2026/09/07/00/00/12/1_p0_master1200.jpg", false},
		{"空地址", "", false},
		{"无法解析", "://invalid", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsImageURL(tt.in))
		})
	}
}

// TestIsImageURLRejectsNonArtworkSegmentsForFromURL 断言两类判读的边界：
// 头像可识别为图片，但不具备可重建的各尺寸结构。
func TestIsImageURLRejectsNonArtworkSegmentsForFromURL(t *testing.T) {
	const profileURL = "https://i.pximg.net/user-profile/img/2022/09/23/01/34/52/23368434_0daa45f98a51e102a4ef48411bffe087_50.jpg"
	require.True(t, IsImageURL(profileURL), "头像应被识别为图片")
	_, err := FromURL(profileURL)
	require.Error(t, err, "头像没有可重建的各尺寸地址")
}

// TestIsImageURLAndFromURLCoverDifferentSets 断言两类判读的边界：
// 可识别为图片的段与可重建各尺寸的段不是同一集合。
//
// 这条约束使「哪些段可重建」必须由本包唯一持有，不能各自写死一份：
// 若把非画作的段误当作可重建，FromURL 会为一个并不存在的尺寸结构
// 编造出地址，而不是报错。
func TestIsImageURLAndFromURLCoverDifferentSets(t *testing.T) {
	// 各类别的实测地址：可识别为图片，但只有画作那三类有各尺寸结构。
	for _, tt := range []struct {
		name        string
		in          string
		rebuildable bool
	}{
		{"画作", "https://i.pximg.net/img-master/img/2026/09/07/00/00/12/149365161_p0_master1200.jpg", true},
		{"画作原图", "https://i.pximg.net/img-original/img/2026/09/07/00/00/12/149365161_p0.png", true},
		{"自定义裁剪", "https://i.pximg.net/c/250x250_80_a2/custom-thumb/img/2026/09/07/00/00/12/149365161_p0_custom1200.jpg", true},
		{"小说封面", "https://i.pximg.net/novel-cover-original/img/2021/01/10/22/47/21/tei14736_2b060b6d13271530d5439f9dbdfe81af.png", false},
		{"作者头像", "https://i.pximg.net/user-profile/img/2022/09/23/01/34/52/23368434_0daa45f98a51e102a4ef48411bffe087_50.jpg", false},
		{"用户背景图", "https://i.pximg.net/background/img/2021/01/10/22/47/21/abc.jpg", false},
		{"动图 zip", "https://i.pximg.net/img-zip-ugoira/img/2026/09/07/00/00/12/149365161_p0_ugoira600x600.zip", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, IsImageURL(tt.in), "应被识别为可取回的 pixiv CDN 资源")
			_, err := FromURL(tt.in)
			if tt.rebuildable {
				require.NoError(t, err, "画作类地址应有各尺寸结构")
				return
			}
			require.Error(t, err, "无各尺寸结构的类别不应被重建")
		})
	}

	// 可重建的段集合确实是全部段集合的真子集。
	all, artwork := PathSegments(), ArtworkPathSegments()
	require.NotEmpty(t, artwork)
	require.Less(t, len(artwork), len(all), "可重建的段应是全部段的一部分")
	for _, seg := range artwork {
		require.Contains(t, all, seg, "派生结果必须来自全部段集合")
	}

	// 返回副本：调用者改动不影响本包判读。
	artwork[0] = "mutated"
	require.NotContains(t, ArtworkPathSegments(), "mutated")
	all[0] = "mutated"
	require.NotContains(t, PathSegments(), "mutated")
}
