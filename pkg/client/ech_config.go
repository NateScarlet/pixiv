package client

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
)

// 本文件实现 ECHConfigList 的编码与「用一份无效配置自举取回可用配置」所需的构造。
//
// 编码格式为 draft-ietf-tls-esni-18，Go 的 crypto/tls 即按该格式解析：
//
//   - 客户端 Config.EncryptedClientHelloConfigList 经 parseECHConfigList 解析，
//     只读一个位于列表最前的 uint16 长度（须等于剩余字节数），随后靠每条配置
//     自身的 version+length 头按 configLen+4 推进——条目之间不得再有长度前缀。
//   - 服务端 EncryptedClientHelloKey.Config 经 parseECHConfig 直接解析，
//     即「裸条目」：version+length+body，不带列表长度前缀。
//
// 因此本文件同时提供两种形态：裸条目用于本地测试的服务端，
// 列表用于客户端。

// ECH 常量。取值来自 RFC 9180（HPKE）与 draft-ietf-tls-esni-18。
const (
	// echConfigVersion 是 ECHConfig 的版本号（即 TLS 扩展类型 encrypted_client_hello）。
	echConfigVersion = 0xfe0d
	// echKEMX25519 是 DHKEM(X25519, HKDF-SHA256)。
	echKEMX25519 = 0x0020
	// echKDFHKDFSHA256 是 HKDF-SHA256。
	echKDFHKDFSHA256 = 0x0001
	// echAEADAES128GCM 是 AES-128-GCM。
	echAEADAES128GCM = 0x0001
	// echMaxNameLength 用 255 表示「不填充内层 ClientHello」。
	//
	// 该字段只影响内层 ClientHello 的域名填充长度，crypto/tls 在解析与选用配置时
	// 都不校验它；取 255 与 Go 自带测试向量一致，也避免取值过小导致内层域名出错。
	echMaxNameLength = 0xff
)

// marshalECHConfig 编码一条裸 ECHConfig（version+length+body）。
//
// publicKey 必须是该 KEM 下合法的公钥点：非法点会被 crypto/tls 在选用配置时跳过，
// 表现为「配置被忽略」而不是报错，故本函数只做长度校验，点的合法性由调用方保证。
func marshalECHConfig(configID uint8, publicKey []byte, publicName string) ([]byte, error) {
	if len(publicName) == 0 || len(publicName) > 255 {
		return nil, fmt.Errorf("pixiv: client: ECH 外层名长度 %d 超出 1～255", len(publicName))
	}
	if len(publicKey) == 0 || len(publicKey) > 0xffff {
		return nil, fmt.Errorf("pixiv: client: ECH 公钥长度 %d 超出 1～65535", len(publicKey))
	}

	var body []byte
	body = append(body, configID)
	body = binary.BigEndian.AppendUint16(body, echKEMX25519)
	body = binary.BigEndian.AppendUint16(body, uint16(len(publicKey)))
	body = append(body, publicKey...)
	// cipher_suites：每项 4 字节的 (kdf_id, aead_id) 对，此处给出一组受支持的组合。
	body = binary.BigEndian.AppendUint16(body, 4)
	body = binary.BigEndian.AppendUint16(body, echKDFHKDFSHA256)
	body = binary.BigEndian.AppendUint16(body, echAEADAES128GCM)
	body = append(body, echMaxNameLength)
	body = append(body, byte(len(publicName)))
	body = append(body, publicName...)
	// extensions 字段必须存在（可以为空），缺失会被 crypto/tls 判为非法。
	body = binary.BigEndian.AppendUint16(body, 0)

	out := make([]byte, 0, len(body)+4)
	out = binary.BigEndian.AppendUint16(out, echConfigVersion)
	out = binary.BigEndian.AppendUint16(out, uint16(len(body)))
	return append(out, body...), nil
}

// marshalECHConfigList 把若干条裸 ECHConfig 拼成一个 ECHConfigList，
// 即客户端 Config.EncryptedClientHelloConfigList 所需的形态。
func marshalECHConfigList(configs ...[]byte) []byte {
	var body []byte
	for _, c := range configs {
		body = append(body, c...)
	}
	out := make([]byte, 0, len(body)+2)
	out = binary.BigEndian.AppendUint16(out, uint16(len(body)))
	return append(out, body...)
}

// bootstrapECHConfigList 生成一份用于自举的 ECHConfigList。
//
// 它的结构完全合法，但公钥是一对临时生成的 X25519 密钥的公钥——服务端不持有对应
// 私钥，因而无法解密内层 ClientHello，会在 HelloRetryRequest 中下发真正的
// retry_configs。这正是 issue 要求的「不依赖 DNS 取得配置」的途径：
// 发送一份无法解密的配置，用服务端的拒绝响应换回可用配置。
//
// 公钥必须是合法的 X25519 点，否则 crypto/tls 会在本地跳过该配置，
// 请求根本发不出去（表现为「没有可用配置」而不是被拒绝）。
func bootstrapECHConfigList(publicName string) ([]byte, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("pixiv: client: 生成 ECH 自举密钥失败: %w", err)
	}
	config, err := marshalECHConfig(0, key.PublicKey().Bytes(), publicName)
	if err != nil {
		return nil, err
	}
	return marshalECHConfigList(config), nil
}

// ErrECHRejected 表示服务端拒绝了 ECH，且未下发可重试的配置。
//
// 这是协议中有意义的信号：服务端拒绝但不下发 retry_configs，说明它不接受 ECH，
// 继续重试没有意义。调用者可用 errors.Is 判定并据此改用其它传输方式。
//
// 需要注意该错误无法区分成因——目标主机不在 Cloudflare 之后、服务端不启用 ECH、
// 或网络中间设备改写了握手，都会走到这里，因为协议本身不携带拒绝原因。
// 因此错误信息只陈述事实，不断言成因。
var ErrECHRejected = errors.New("服务端拒绝 ECH 且未提供可用配置")
