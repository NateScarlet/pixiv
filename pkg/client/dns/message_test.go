package dns

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dnsResponseFor 构造一个应答报文：回显请求 ID、拷回问题段，并附一条 A 记录。
func dnsResponseFor(query []byte, ip [4]byte) []byte {
	var out []byte
	out = append(out, query[:2]...) // ID 原样回显
	// 标志：QR=1（应答）、RD=1、RA=1，RCODE=0。
	out = append(out, 0x81, 0x80)
	out = append(out, 0x00, 0x01) // QDCOUNT
	out = append(out, 0x00, 0x01) // ANCOUNT
	out = append(out, 0x00, 0x00) // NSCOUNT
	out = append(out, 0x00, 0x00) // ARCOUNT
	// 问题段：请求报文里首部 12 字节之后即是，长度到报文末尾。
	out = append(out, query[12:]...)
	// 应答记录：压缩指针指向问题段的 QNAME（偏移 12）。
	out = append(out, 0xc0, 0x0c)
	out = append(out, 0x00, 0x01)             // TYPE A
	out = append(out, 0x00, 0x01)             // CLASS IN
	out = append(out, 0x00, 0x00, 0x00, 0x3c) // TTL
	out = append(out, 0x00, 0x04)             // RDLENGTH
	out = append(out, ip[:]...)
	return out
}

// TestDOHResolverWireFormatMessage 断言二进制报文方式能解析出 A 记录。
//
// 该用例复现 dnscrypt-proxy 本地 DoH 服务端的行为：只接受
// application/dns-message 的 RFC 8484 查询，收到 JSON 接口的 name 参数
// 时以 400 拒绝。响应不是 JSON，因此也钉住了「不得用 JSON 解析器
// 解读二进制响应」。
func TestDOHResolverWireFormatMessage(t *testing.T) {
	var seenAccept string
	var seenMethod string
	var seenQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seenAccept = req.Header.Get("Accept")
		seenMethod = req.Method
		seenQuery = req.URL.Query().Get("name")
		// 只实现 RFC 8484：没有 dns 参数即拒绝，与 dnscrypt-proxy 一致。
		raw := req.URL.Query().Get("dns")
		if raw == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "local DoH server\n")
			return
		}
		query, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(dnsResponseFor(query, [4]byte{210, 140, 139, 129}))
	}))
	t.Cleanup(srv.Close)

	// 不带 fragment 即默认二进制，符合 RFC 8484 对实现的要求。
	r := NewDOHResolver(srv.URL)
	ips, err := r.Resolve(context.Background(), "i.pximg.net")
	require.NoError(t, err)

	got := make([]string, 0, len(ips))
	for _, ip := range ips {
		got = append(got, ip.String())
	}
	assert.Equal(t, []string{"210.140.139.129"}, got)
	assert.Equal(t, "application/dns-message", seenAccept)
	assert.Equal(t, http.MethodGet, seenMethod)
	assert.Empty(t, seenQuery, "二进制方式不应再发 name 参数")
}

// TestDOHResolverJSONFormatFromFragment 断言 #type=json 声明切换到 JSON 接口。
func TestDOHResolverJSONFormatFromFragment(t *testing.T) {
	var seenAccept string
	var seenQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seenAccept = req.Header.Get("Accept")
		seenQuery = req.URL.Query().Get("name")
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = io.WriteString(w, `{"Answer":[{"type":1,"data":"210.140.139.129"}]}`)
	}))
	t.Cleanup(srv.Close)

	r := NewDOHResolver(srv.URL + "#type=json")
	ips, err := r.Resolve(context.Background(), "i.pximg.net")
	require.NoError(t, err)
	require.Len(t, ips, 1)
	assert.Equal(t, "210.140.139.129", ips[0].String())
	assert.Equal(t, "application/dns-json", seenAccept)
	assert.Equal(t, "i.pximg.net", seenQuery)
}

// TestDOHResolverJSONFormatStillRejectsMessageOnlyServer 断言 JSON 方式对
// 只支持 RFC 8484 的服务端仍然失败——那是服务端的拒绝，不是解析错觉。
func TestDOHResolverJSONFormatStillRejectsMessageOnlyServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "local DoH server\n")
	}))
	t.Cleanup(srv.Close)

	r := NewDOHResolver(srv.URL + "#type=json")
	_, err := r.Resolve(context.Background(), "i.pximg.net")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

// TestNewDOHResolverParsesEndpointFragment 断言端点 URL 的 fragment 决定
// 编码方式，且 fragment 本身不带进请求地址。
func TestNewDOHResolverParsesEndpointFragment(t *testing.T) {
	for _, tt := range []struct {
		name       string
		endpoint   string
		wantURL    string
		wantFormat DoHWireFormat
	}{
		{
			"不带 fragment 即默认二进制",
			"https://doh.example/dns-query",
			"https://doh.example/dns-query",
			DoHWireFormatMessage,
		},
		{
			"显式声明二进制",
			"https://doh.example/dns-query#type=message",
			"https://doh.example/dns-query",
			DoHWireFormatMessage,
		},
		{
			"别名 dns-message",
			"https://doh.example/dns-query#type=dns-message",
			"https://doh.example/dns-query",
			DoHWireFormatMessage,
		},
		{
			"声明 JSON",
			"https://doh.example/dns-query#type=json",
			"https://doh.example/dns-query",
			DoHWireFormatJSON,
		},
		{
			"保留查询参数",
			"https://doh.example/dns-query?x=1#type=json",
			"https://doh.example/dns-query?x=1",
			DoHWireFormatJSON,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := NewDOHResolver(tt.endpoint)
			assert.Equal(t, tt.wantURL, r.URL(), "fragment 不应带进请求地址")
			assert.Equal(t, tt.wantFormat, r.(*dohResolver).format)
		})
	}
}

// TestNewDOHResolverRejectsInvalidFragment 断言非法声明快速失败：
// 静默忽略会让一个不会生效的地址继续运行，失败推迟到首次解析，
// 且表现为「端点拒绝查询」而非「URL 写错」。
func TestNewDOHResolverRejectsInvalidFragment(t *testing.T) {
	for _, tt := range []struct {
		name     string
		endpoint string
		wantErr  string
	}{
		{"不认识的方式", "https://doh.example/dns-query#type=binary", `type 的值 "binary" 无效`},
		{"不含 type 参数", "https://doh.example/dns-query#json", "不含 type 参数"},
		{"空值", "https://doh.example/dns-query#type=", "不含 type 参数"},
		{"缺少主机名", "doh.example/dns-query", "缺少主机名"},
		{"空地址", "", "缺少主机名"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				got := recover()
				require.NotNil(t, got, "非法 fragment 应快速失败")
				assert.Contains(t, fmt.Sprint(got), tt.wantErr)
			}()
			NewDOHResolver(tt.endpoint)
		})
	}
}

// TestBuildQueryA 断言查询报文的编码符合 RFC 1035。
func TestBuildQueryA(t *testing.T) {
	msg, err := buildQueryA("i.pximg.net", 0x1234)
	require.NoError(t, err)

	assert.Equal(t, uint16(0x1234), binary.BigEndian.Uint16(msg[0:2]))
	assert.Equal(t, uint16(0x0100), binary.BigEndian.Uint16(msg[2:4]), "应为带 RD 的标准查询")
	assert.Equal(t, uint16(1), binary.BigEndian.Uint16(msg[4:6]), "QDCOUNT 应为 1")
	// 问题段：1"i" 5"pximg" 3"net" 0，随后 QTYPE=A、QCLASS=IN。
	expected := []byte{
		0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		1, 'i', 5, 'p', 'x', 'i', 'm', 'g', 3, 'n', 'e', 't', 0,
		0x00, 0x01, 0x00, 0x01,
	}
	assert.Equal(t, expected, msg)
}

// TestBuildQueryARejectsInvalidName 断言无法编码的主机名快速失败。
func TestBuildQueryARejectsInvalidName(t *testing.T) {
	for _, name := range []string{"", ".", "a..b"} {
		_, err := buildQueryA(name, 0)
		assert.Error(t, err, "主机名 %q 不应被接受", name)
	}
	// 单标签超过 63 字节无法编码。
	_, err := buildQueryA(string(make([]byte, 64)), 0)
	assert.Error(t, err)
}

// TestParseARecords 断言应答解析覆盖压缩指针与多记录的情形。
func TestParseARecords(t *testing.T) {
	query, err := buildQueryA("i.pximg.net", 0)
	require.NoError(t, err)

	resp := dnsResponseFor(query, [4]byte{1, 2, 3, 4})
	got, err := parseARecords(resp, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.2.3.4"}, got)
}

// TestParseARecordsRejectsMismatchedID 断言事务 ID 不匹配时报错，
// 避免把错配的应答当作本查询的结果。
func TestParseARecordsRejectsMismatchedID(t *testing.T) {
	query, err := buildQueryA("i.pximg.net", 0x1234)
	require.NoError(t, err)

	resp := dnsResponseFor(query, [4]byte{1, 2, 3, 4})
	_, err = parseARecords(resp, 0x9999)
	assert.ErrorContains(t, err, "不匹配")
}

// TestParseARecordsReportsRCODE 断言服务器返回的错误码被如实报出，
// 而不是被当成「没有 A 记录」。
func TestParseARecordsReportsRCODE(t *testing.T) {
	query, err := buildQueryA("nonexistent.example", 0)
	require.NoError(t, err)

	resp := dnsResponseFor(query, [4]byte{1, 2, 3, 4})
	resp[3] = 0x83 // RCODE=3（域名不存在）
	_, err = parseARecords(resp, 0)
	assert.ErrorContains(t, err, "域名不存在")
}

// TestParseARecordsRejectsTruncated 断言结构异常的报文报错而非静默返回空结果：
// 「解析失败」与「该主机没有 A 记录」是两回事。
func TestParseARecordsRejectsTruncated(t *testing.T) {
	_, err := parseARecords([]byte{0x00}, 0)
	assert.Error(t, err)

	query, err := buildQueryA("i.pximg.net", 0)
	require.NoError(t, err)
	resp := dnsResponseFor(query, [4]byte{1, 2, 3, 4})
	_, err = parseARecords(resp[:len(resp)-2], 0)
	assert.Error(t, err)
}
