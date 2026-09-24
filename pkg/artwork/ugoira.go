package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"time"

	"github.com/NateScarlet/pixiv/pkg/client"
	"github.com/tidwall/gjson"
)

// FetchUgoiraMeta 获取动图作品的元数据：zip 地址与帧时序。
//
// 仅对动画作品（illustType=2）有效；其它类型该接口返回错误信封，
// 本方法会以错误形式给出。调用者需要先区分作品类型时，可自行用 [Fetch] 查
// [FetchPayload.Type]。
//
// 下载动图即取回 [UgoiraMeta.ZipURL]（或 [UgoiraMeta.OriginalZipURL]）指向的
// zip：zip 与图片同主机、同样要求 Referer，用 [client.Client.FetchImage] 取回
// 即可，本库不为此提供重复的下载方法。
func FetchUgoiraMeta(ctx context.Context, id string) (_ UgoiraMeta, err error) {
	if id == "" {
		err = errors.New("pixiv: artwork.FetchUgoiraMeta: id is required")
		return
	}
	var c = client.For(ctx)
	resp, err := c.GetWithContext(ctx, c.EndpointURL("/ajax/illust/"+id+"/ugoira_meta", nil).String())
	if err != nil {
		return
	}
	body, err := client.ParseAPIResponseV2(resp)
	if err != nil {
		return
	}
	return UgoiraMeta{id, body, resp}, nil
}

// UgoiraMeta 是动图作品的元数据（zip 地址与帧时序），不可变记录。
// UgoiraMeta is an immutable record of an animated artwork's metadata.
type UgoiraMeta struct {
	id   string
	raw  json.RawMessage
	resp *http.Response
}

func (m UgoiraMeta) ID() string {
	return m.id
}

func (m UgoiraMeta) Raw() json.RawMessage {
	return m.raw
}

func (m UgoiraMeta) Response() *http.Response {
	return m.resp
}

func (m UgoiraMeta) get(path string) gjson.Result {
	return gjson.GetBytes(m.raw, path)
}

// ZipURL returns the compressed zip url (src), the fallback available without login.
// ZipURL 返回压缩版动图 zip 地址（src 字段），未登录时通常可用的兜底。
func (m UgoiraMeta) ZipURL() string {
	return m.get("src").String()
}

// OriginalZipURL returns the original resolution zip url (originalSrc), fetching usually requires login.
// OriginalZipURL 返回原分辨率动图 zip 地址（originalSrc 字段），取回通常需要登录。
func (m UgoiraMeta) OriginalZipURL() string {
	return m.get("originalSrc").String()
}

// MimeType returns the animation format, e.g. image/gif.
// MimeType 返回动图格式，如 image/gif。
func (m UgoiraMeta) MimeType() string {
	return m.get("mime_type").String()
}

// Frames returns an iterator of frames in playback order.
// Frames 按播放顺序返回帧的迭代器；zip 内文件名与 [UgoiraFrame.Delay] 由调用者
// 用于拼装帧序列（如转成 GIF 时给出每帧时长）。
func (m UgoiraMeta) Frames() iter.Seq[UgoiraFrame] {
	return func(yield func(UgoiraFrame) bool) {
		m.get("frames").ForEach(func(_, value gjson.Result) bool {
			return yield(UgoiraFrame{
				File:  value.Get("file").String(),
				Delay: time.Duration(value.Get("delay").Int()) * time.Millisecond,
			})
		})
	}
}

// UgoiraFrame is a single frame of an animated artwork.
// UgoiraFrame 是动图作品中的一帧。
type UgoiraFrame struct {
	// File is the frame filename inside the zip, in playback order.
	// File 是帧在 zip 内的文件名，按播放顺序排列。
	File string
	// Delay is the display duration of this frame.
	// Delay 是该帧的显示时长。
	Delay time.Duration
}
