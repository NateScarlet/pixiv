package novel

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

var imagePattern = regexp.MustCompile(`\[pixivimage:(\w+)(?:-(\d+))?\]`)

// ContentRenderer can render novel content
type ContentRenderer interface {
	Image(ctx context.Context, id string, index int) (string, error)
	Paragraph(ctx context.Context, text string) (string, error)
	PageBreak(ctx context.Context) (string, error)
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

// PageBreak implements ContentRenderer
func (r SimpleContentRenderer) PageBreak(ctx context.Context) (ret string, err error) {
	return "<hr>", nil
}

// HTMLContent render novel content to html.
func HTMLContent(ctx context.Context, renderer ContentRenderer, content string) (ret string, err error) {
	ret = template.HTMLEscapeString(content)
	lines := strings.Split(ret, "\n")
	for index, line := range lines {
		if line == "[newpage]" {
			lines[index], err = renderer.PageBreak(ctx)
		} else {
			lines[index], err = renderer.Paragraph(ctx, line)
		}
		if err != nil {
			return
		}
	}
	ret = strings.Join(lines, "\n")
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
	return
}
