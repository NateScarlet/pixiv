package novel

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var newPagePattern = regexp.MustCompile(`\[newpage]`)
var imagePattern = regexp.MustCompile(`\[pixivimage:(\w+?)(?:-(\d+?))?]`)
var rubyPattern = regexp.MustCompile(`\[\[rb:(.+?)? ?> ?(.+?)]]`)
var chapterPattern = regexp.MustCompile(`\[chapter:(.+?)?]`)
var jumpURIPattern = regexp.MustCompile(`\[\[jumpuri:(.+?)? ?> ?(.+?)]]`)
var jumpPagePattern = regexp.MustCompile(`\[jump:(\d+?)?]`)

// ContentRenderer can render novel content
type ContentRenderer interface {
	Image(ctx context.Context, id string, index int) (string, error)
	Paragraph(ctx context.Context, text string) (string, error)
	NewPage(ctx context.Context, index int) (string, error)
	Ruby(ctx context.Context, ruby, rt string) (string, error)
	Chapter(ctx context.Context, name string) (string, error)
	JumpURI(ctx context.Context, title, uri string) (string, error)
	JumpPage(ctx context.Context, page int) (string, error)
}

// SimpleContentRenderer is a simple implementation of ContentRenderer
type SimpleContentRenderer struct{}

// Image implements ContentRenderer
func (r SimpleContentRenderer) Image(ctx context.Context, id string, index int) (ret string, err error) {
	if index < 1 {
		return fmt.Sprintf(`<img src=" https://pixiv.cat/%s.jpg" />`, id), nil
	}
	return fmt.Sprintf(`<img src=" https://pixiv.cat/%s-%d.jpg" />`, id, index), nil
}

// Paragraph implements ContentRenderer
func (r SimpleContentRenderer) Paragraph(ctx context.Context, text string) (ret string, err error) {
	return fmt.Sprintf(`<p>%s</p>`, text), nil
}

// NewPage implements ContentRenderer
func (r SimpleContentRenderer) NewPage(ctx context.Context, index int) (ret string, err error) {
	return fmt.Sprintf(`<hr id="page-%d">`, index), nil
}

// Ruby implements ContentRenderer
func (r SimpleContentRenderer) Ruby(ctx context.Context, ruby, rt string) (string, error) {
	return fmt.Sprintf(`<ruby>%s<rt>%s</rt></ruby>`, ruby, rt), nil
}

// Chapter implements ContentRenderer
func (r SimpleContentRenderer) Chapter(ctx context.Context, name string) (ret string, err error) {
	return fmt.Sprintf(`<h2>%s</h2>`, name), nil
}

// JumpURI implements ContentRenderer
func (r SimpleContentRenderer) JumpURI(ctx context.Context, title, uri string) (ret string, err error) {
	return fmt.Sprintf(`<a href="%s">%s</a>`, uri, title), nil
}

// JumpPage implements ContentRenderer
func (r SimpleContentRenderer) JumpPage(ctx context.Context, index int) (ret string, err error) {
	return fmt.Sprintf(`<a href="#page-%d">page %d</a>`, index, index), nil
}

func htmlContentLine(ctx context.Context, renderer ContentRenderer, line string, pageIndex *int) (ret string, err error) {
	ret = line
	ret = newPagePattern.ReplaceAllStringFunc(ret, func(s string) (ret string) {
		*pageIndex++
		ret, err = renderer.NewPage(ctx, *pageIndex)
		return
	})
	if err != nil {
		return
	}
	ret = rubyPattern.ReplaceAllStringFunc(
		ret,
		func(v string) (ret string) {
			if err != nil {
				return
			}
			match := rubyPattern.FindStringSubmatch(v)
			ret, err = renderer.Ruby(ctx, match[1], match[2])
			return
		},
	)
	if err != nil {
		return
	}
	ret = imagePattern.ReplaceAllStringFunc(
		ret,
		func(v string) (ret string) {
			if err != nil {
				return
			}
			match := imagePattern.FindStringSubmatch(v)
			id := match[1]
			index, _ := strconv.Atoi(match[2])
			ret, err = renderer.Image(ctx, id, index)
			return
		},
	)
	if err != nil {
		return
	}
	ret = chapterPattern.ReplaceAllStringFunc(
		ret,
		func(v string) (ret string) {
			if err != nil {
				return
			}
			match := chapterPattern.FindStringSubmatch(v)
			ret, err = renderer.Chapter(ctx, match[1])
			return
		},
	)
	if err != nil {
		return
	}
	ret = jumpURIPattern.ReplaceAllStringFunc(
		ret,
		func(v string) (ret string) {
			if err != nil {
				return
			}
			match := jumpURIPattern.FindStringSubmatch(v)
			ret, err = renderer.JumpURI(ctx, match[1], match[2])
			return
		},
	)
	if err != nil {
		return
	}
	ret = jumpPagePattern.ReplaceAllStringFunc(
		ret,
		func(v string) (ret string) {
			if err != nil {
				return
			}
			match := jumpPagePattern.FindStringSubmatch(v)
			var index int
			index, err = strconv.Atoi(match[1])
			if err != nil {
				return
			}
			ret, err = renderer.JumpPage(ctx, index)
			return
		},
	)
	if err != nil {
		return
	}

	return
}

// HTMLContent render novel content to html.
// format from https://www.pixiv.net/novel/upload.php
func HTMLContent(ctx context.Context, renderer ContentRenderer, content string) (ret string, err error) {
	ret = content
	lines := strings.Split(ret, "\n")
	var pageIndex = 0
	for index, line := range lines {
		lines[index], err = htmlContentLine(ctx, renderer, line, &pageIndex)
		if err != nil {
			return
		}
	}
	ret = strings.Join(lines, "\n")
	return
}
