package security

import (
	"encoding/hex"
	"regexp"
)

// ============================================================
// 内置病毒特征库
//   说明: 特征串以 XOR(0x5A) 编码存放, 运行时解码。
//   原因: 若源码/二进制中直接出现 EICAR 等特征串, 引擎自身会被
//         杀毒软件误报（实测 Windows Defender 会直接隔离编译产物）,
//         因此采用「编码存放 + 运行时解码」的方式装载特征库。
// ============================================================

// eicarEncoded EICAR 标准反病毒测试样本的 XOR(0x5A) 十六进制编码
const eicarEncoded = "026f157b0a7f1a1b0a016e060a00026f6e720a04736d1919736d277e1f13191b0877090e1b141e1b081e771b140e130c13080f09770e1f090e771c13161f7b7e12711270"

// xorDecode 解码特征串
func xorDecode(encoded string) string {
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return ""
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = b ^ 0x5A
	}
	return string(out)
}

// MalwareSignatureEICAR 返回 EICAR 测试样本特征串 (供引擎与自测使用)
func MalwareSignatureEICAR() string { return xorDecode(eicarEncoded) }

// eicarPattern EICAR 匹配规则 (转义后用于正则匹配)
var eicarPattern = regexp.MustCompile(regexp.QuoteMeta(xorDecode(eicarEncoded)))

// signatureCount 内置病毒特征串数量
func signatureCount() int { return 1 }
