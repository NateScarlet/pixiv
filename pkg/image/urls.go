package image

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// URLs contains image url for different size.
type URLs struct {
	// max 48px
	Mini string
	// max 250px
	Thumb string
	// max 540px
	Small string
	// Master size (max 1200px)
	Regular string
	// Original size
	Original string
}

// FromURL reconstructs all size fields from a single pixiv image URL.
// 从任意 pixiv 图片 URL 重建各尺寸图片 URL。
//
// It scans the path with Sscanf to recognize the optional `/c/{size}/` prefix, the
// path segment (img-master / custom-thumb / img-original) and the date plus filename
// segments, then fills Mini / Thumb / Small / Regular / Original.
// 通过 Sscanf 扫描路径中的 `/c/{size}/` 前缀、图片类型段以及日期和文件名片段，填充各尺寸字段。
//
// The original field keeps the extension of the input URL, which may differ from the
// real original (e.g. transparency / large artworks are usually PNG).
// 说明：original 字段沿用输入 URL 的扩展名，真实原图扩展名可能不同(PNG/大图)。
//
// It returns an error carrying the input URL when the URL cannot be recognized.
// 若无法识别(非 pixiv 图片结构)则返回携带输入 URL 的错误。
func FromURL(rawURL string) (_ URLs, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("pixiv: image.FromURL: %w", err)
		}
	}()

	u, err := url.Parse(rawURL)
	if err != nil {
		return URLs{}, err
	}
	if u.Scheme == "" || u.Host == "" {
		return URLs{}, unrecognized(rawURL)
	}

	// 先剥离可选的 /c/{size}/ 前缀(缩略图)，其余部分结构固定:
	// {pathSeg}/img/{y}/{M}/{d}/{H}/{min}/{s}/{file}
	p := strings.Trim(u.Path, "/")
	if strings.HasPrefix(p, "c/") {
		p = strings.TrimPrefix(p, "c/")
		slash := strings.Index(p, "/")
		if slash < 0 {
			return URLs{}, unrecognized(rawURL)
		}
		p = p[slash+1:] // 去掉 {size}
	}

	// fmt 的 %s 只在遇到空白时停止，无法直接按 '/' 分割路径，
	// 因此先把路径切成段，再用单个空格拼接，之后即可按固定位置用 Sscanf 扫描。
	tokens := strings.Join(strings.Split(p, "/"), " ")

	var (
		pathSeg string
		ymdhms  [6]string
		file    string
	)

	// img-master/img/2026/08/26/00/00/29/148882180_p0_master1200.jpg
	if n, _ := fmt.Sscanf(tokens,
		"%s img %s %s %s %s %s %s %s",
		&pathSeg, &ymdhms[0], &ymdhms[1], &ymdhms[2], &ymdhms[3], &ymdhms[4], &ymdhms[5], &file,
	); n != 8 {
		return URLs{}, unrecognized(rawURL)
	}

	switch pathSeg {
	case "img-master", "custom-thumb", "img-original":
	default:
		return URLs{}, unrecognized(rawURL)
	}

	// 拆出基础文件名与扩展名: 148882180_p0_master1200.jpg
	dot := strings.LastIndexByte(file, '.')
	if dot <= 0 || dot == len(file)-1 {
		return URLs{}, unrecognized(rawURL)
	}
	base, ext := file[:dot], file[dot+1:]

	// 去掉可能的尺寸后缀，得到 作品ID_p页码 的基础名
	for _, suffix := range []string{"_master1200", "_square1200", "_custom1200"} {
		if strings.HasSuffix(base, suffix) {
			base = strings.TrimSuffix(base, suffix)
			break
		}
	}

	imgPath := "/img/" + strings.Join(ymdhms[:], "/") + "/"
	return URLs{
		// 缩略图统一使用 img-master 段(custom-thumb 为作者自定义裁剪，非标准尺寸)
		Mini:     cdnURL(u, "/c/48x48/img-master"+imgPath+base+"_square1200."+ext),
		Thumb:    cdnURL(u, "/c/250x250_80_a2/img-master"+imgPath+base+"_square1200."+ext),
		Small:    cdnURL(u, "/c/540x540_70/img-master"+imgPath+base+"_master1200."+ext),
		Regular:  cdnURL(u, "/img-master"+imgPath+base+"_master1200."+ext),
		Original: cdnURL(u, "/img-original"+imgPath+base+"."+ext),
	}, nil
}

// cdnURL 用传入 URL 的协议与主机拼接出新的 CDN 图片地址
func cdnURL(base *url.URL, path string) string {
	return (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: path}).String()
}

// unrecognized 构造携带输入 URL 的不可识别错误
func unrecognized(rawURL string) error {
	return errors.New("unrecognized image URL " + strconv.Quote(rawURL))
}
