package dns

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// dnsMessage 是一次查询报文的构造与响应解析。
//
// 这里只处理本解析器需要的部分：单个 A 记录查询与其应答中的 A 记录地址。
// 报文格式见 RFC 1035，DoH 的二进制载体见 RFC 8484。

// dnsTypeA 是 A 记录的 QTYPE/TYPE 值。
const dnsTypeA = 1

// dnsClassIN 是 Internet 类的 QCLASS 值。
const dnsClassIN = 1

// buildQueryA 构造一个查询 A 记录的 DNS 报文，并返回其事务 ID。
//
// id 用于把响应与请求对应起来（RFC 1035 4.1.1）：解析器发出的请求各自独立，
// 校验响应 ID 可以排除应答错配。
func buildQueryA(name string, id uint16) ([]byte, error) {
	labels, err := splitLabels(name)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	// 首部 12 字节：ID、标志、四个计数字段。
	var header [12]byte
	binary.BigEndian.PutUint16(header[0:2], id)
	// 标志：QR=0（查询），OPCODE=0（标准查询），RD=1（期望递归）。
	binary.BigEndian.PutUint16(header[2:4], 0x0100)
	binary.BigEndian.PutUint16(header[4:6], 1) // QDCOUNT
	// ANCOUNT / NSCOUNT / ARCOUNT 保持 0。
	buf.Write(header[:])

	// 问题段：QNAME + QTYPE + QCLASS。
	for _, label := range labels {
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}
	buf.WriteByte(0) // QNAME 以根标签结束
	var tail [4]byte
	binary.BigEndian.PutUint16(tail[0:2], dnsTypeA)
	binary.BigEndian.PutUint16(tail[2:4], dnsClassIN)
	buf.Write(tail[:])

	return buf.Bytes(), nil
}

// splitLabels 把主机名拆成 DNS 标签。
//
// 各标签长度须在 1..63 之间（RFC 1035 2.3.4），超长或空标签无法编码，
// 快速失败而不是发出一个必然被拒绝的报文。
func splitLabels(name string) ([]string, error) {
	// 末尾的点表示根，编码时由末尾的 0 长度标签表达，这里先去掉。
	trimmed := strings.TrimSuffix(name, ".")
	if trimmed == "" {
		return nil, fmt.Errorf("pixiv: dns: 主机名为空")
	}
	var labels []string
	for _, label := range strings.Split(trimmed, ".") {
		if label == "" {
			return nil, fmt.Errorf("pixiv: dns: 主机名 %q 含空标签", name)
		}
		if len(label) > 63 {
			return nil, fmt.Errorf("pixiv: dns: 主机名 %q 的标签 %q 超过 63 字节", name, label)
		}
		labels = append(labels, label)
	}
	return labels, nil
}

// ParseError 表示端点的响应无法解析为 DNS 报文。
//
// 端点可达且接受了查询，因此与网络错误是两回事：诊断工具据此把
// 「端点不可达」与「响应不合预期」区分开。
type ParseError struct {
	msg string
}

func (e *ParseError) Error() string { return e.msg }

// newParseError 构造一个解析错误。
func newParseError(format string, args ...any) error {
	return &ParseError{msg: fmt.Sprintf(format, args...)}
}

// parseARecords 解析响应报文，取出应答段中的 A 记录地址。
//
// 解析遇到结构异常时返回错误而非静默截断：截断的结果会被当成「该主机不存在」，
// 那与「响应无法解析」是两回事（快速失败）。
func parseARecords(msg []byte, wantID uint16) ([]string, error) {
	if len(msg) < 12 {
		return nil, newParseError("pixiv: dns: 响应报文过短（%d 字节），不构成 DNS 首部", len(msg))
	}
	gotID := binary.BigEndian.Uint16(msg[0:2])
	if gotID != wantID {
		return nil, newParseError("pixiv: dns: 响应 ID %d 与请求 ID %d 不匹配", gotID, wantID)
	}
	// 标志的低 4 位是 RCODE：0 表示无错误。
	if rcode := msg[3] & 0x0f; rcode != 0 {
		return nil, newParseError("pixiv: dns: 服务器返回 RCODE %d（%s）", rcode, rcodeText(rcode))
	}

	qdcount := int(binary.BigEndian.Uint16(msg[4:6]))
	ancount := int(binary.BigEndian.Uint16(msg[6:8]))

	// 跳过问题段后进入应答段。
	offset := 12
	for i := 0; i < qdcount; i++ {
		next, err := skipName(msg, offset)
		if err != nil {
			return nil, err
		}
		// QTYPE + QCLASS 共 4 字节。
		offset = next + 4
		if offset > len(msg) {
			return nil, newParseError("pixiv: dns: 问题段越界")
		}
	}

	var ips []string
	for i := 0; i < ancount; i++ {
		next, err := skipName(msg, offset)
		if err != nil {
			return nil, err
		}
		if next+10 > len(msg) {
			return nil, newParseError("pixiv: dns: 应答记录首部越界")
		}
		rrType := binary.BigEndian.Uint16(msg[next : next+2])
		rdlength := int(binary.BigEndian.Uint16(msg[next+8 : next+10]))
		rdata := next + 10
		if rdata+rdlength > len(msg) {
			return nil, newParseError("pixiv: dns: 应答记录数据越界")
		}
		if rrType == dnsTypeA {
			// A 记录的 RDATA 必须是 4 字节 IPv4 地址。
			if rdlength != 4 {
				return nil, newParseError("pixiv: dns: A 记录长度为 %d，应为 4", rdlength)
			}
			ips = append(ips, fmt.Sprintf("%d.%d.%d.%d",
				msg[rdata], msg[rdata+1], msg[rdata+2], msg[rdata+3]))
		}
		offset = rdata + rdlength
	}
	return ips, nil
}

// skipName 跳过应答中的域名，返回其后的偏移量。
//
// 域名可能以 0x00 结束，也可能以 0xC0 开头的压缩指针结束（RFC 1035 4.1.4）。
// 压缩指针必然指向报文内更早的位置，因此跳过它不会造成循环。
func skipName(msg []byte, offset int) (int, error) {
	for {
		if offset >= len(msg) {
			return 0, newParseError("pixiv: dns: 域名越界")
		}
		length := int(msg[offset])
		switch {
		case length == 0:
			return offset + 1, nil
		case length&0xc0 == 0xc0:
			// 压缩指针占 2 字节，指针本身即域名结束。
			if offset+2 > len(msg) {
				return 0, newParseError("pixiv: dns: 压缩指针越界")
			}
			return offset + 2, nil
		default:
			offset += 1 + length
		}
	}
}

// rcodeText 给出 RCODE 的含义，用于可读的错误信息。
func rcodeText(rcode byte) string {
	switch rcode {
	case 1:
		return "格式错误"
	case 2:
		return "服务器故障"
	case 3:
		return "域名不存在"
	case 4:
		return "查询类型不支持"
	case 5:
		return "服务器拒绝"
	default:
		return "未知"
	}
}
