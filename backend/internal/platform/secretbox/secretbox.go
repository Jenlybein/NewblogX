// Package secretbox 负责"落库的密钥不以明文存在"。
//
// 为什么需要它：站点的运行时配置是数据库里的一段 JSON（管理员在后台改的那份），
// 里面既有普通设置，也有腾讯云 API 密钥、对象存储密钥这类凭据。明文存库意味着
// 一次备份泄漏、一次 SQL 注入、一次误导出就把凭据交出去了；而备份文件往往散落在
// 更多地方（本机、对象存储、别人电脑），风险面比数据库本身大得多。
//
// 设计取舍：**加密落库 + 只写不读**。
//
//	管理员后台照旧可以填写与覆盖密钥（这是日常要用的），但接口永远不回显明文
//	（回显的是占位符），所以"能在后台配"与"库里不存明文"可以同时成立。
//
// 主密钥只来自环境变量 `BLOGX_CONFIG_SECRET_KEY`，不落库、不进代码：
// 它一旦和密文放在同一处，加密就只是换个编码。
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// EnvKey 是主密钥的环境变量名。
const EnvKey = "BLOGX_CONFIG_SECRET_KEY"

// sealedPrefix 标记"这段值已经加密"。
//
// 带前缀而不是靠长度猜：读路径要能同时接受历史明文（升级前写进去的那些），
// 前缀让"要不要解密"变成确定性判断，也让迁移命令可以幂等 —— 已经有前缀的
// 值不会被再加密一次。
const sealedPrefix = "enc:v1:"

// keySize 是 AES-256 的密钥长度。
const keySize = 32

// ErrNoMasterKey 表示没有配置主密钥，因此不能加密落库。
//
// 哨兵错误的文本用英文、只承担"身份"：面向用户与运维的中文文案在 apperr 那一层
// （见 config.secretStoreError 映射到 runtimeconfig.secret_unavailable）。
// 这也是 archcheck R8 的口径——错误码负责文案，内部错误负责识别。
var ErrNoMasterKey = errors.New("secretbox: master key not configured")

// ErrBadCiphertext 表示密文本身不合法（截断、非 base64、非本密钥加密）。
var ErrBadCiphertext = errors.New("secretbox: ciphertext is malformed")

// Box 是一把用于配置密钥的对称加解密器。
type Box struct {
	aead cipher.AEAD
}

// FromEnv 按环境变量构造 Box。
//
// 返回 (nil, nil) 表示"没配主密钥"：调用方据此决定是降级（只读历史明文）
// 还是拒绝写入。这里不直接报错，是因为没有主密钥的部署仍然要能启动 ——
// 拒绝发生在"要写密钥"的那一刻，而不是进程启动时。
func FromEnv() (*Box, error) {
	// 原文形态不能 TrimSpace：随机字节里完全可能出现空格、换行，
	// 裁掉就变成另一把密钥（解不开线上已有的密文）。
	raw := os.Getenv(EnvKey)
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	key, err := decodeKey(raw)
	if err != nil {
		return nil, err
	}
	return New(key)
}

// New 用给定密钥构造 Box。
func New(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secretbox: 密钥长度必须是 %d 字节，实际 %d", keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: 初始化 AES 失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: 初始化 GCM 失败: %w", err)
	}
	return &Box{aead: aead}, nil
}

// decodeKey 接受三种常见写法：base64、hex、以及 32 个可打印字符的原文。
//
// 三种都认是为了让人能直接用 `openssl rand -base64 32` 的输出，不用先转格式 ——
// 运维少一步转换，就少一次"配错了但没发现"。
func decodeKey(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if b, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(b) == keySize {
		return b, nil
	}
	if b, err := hex.DecodeString(trimmed); err == nil && len(b) == keySize {
		return b, nil
	}
	// 原文形态用**未裁剪**的值：随机字节里的空白是密钥的一部分。
	if len(raw) == keySize {
		return []byte(raw), nil
	}
	return nil, fmt.Errorf("secretbox: %s 不是可用的密钥（需要 32 字节，可 base64 / hex / 原文）", EnvKey)
}

// IsSealed 判断一个值是否已经加密。
func IsSealed(value string) bool {
	return strings.HasPrefix(value, sealedPrefix)
}

// Seal 加密一个明文值。
//
// 空串原样返回：空值没有需要保护的内容，加密它只会让"未配置"与"配置了空值"
// 变得无法区分。
func (b *Box) Seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if b == nil {
		return "", ErrNoMasterKey
	}
	if IsSealed(plain) {
		// 已经是密文（管理员原样回传了占位符之外的东西时可能发生）：幂等。
		return plain, nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("secretbox: 生成随机数失败: %w", err)
	}
	// AAD 固定为空：密文与"它属于哪个字段"的绑定由配置段结构承担，
	// 挪动密文只会解不开（GCM 校验失败），不会解出别人的值。
	sealed := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return sealedPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Open 解密一个值。
//
// 没有前缀的值原样返回：那是升级前写进去的历史明文，读路径必须继续接受它，
// 否则升级当天邮件、上传就全停了。把它转成密文是迁移命令的职责。
func (b *Box) Open(value string) (string, error) {
	if !IsSealed(value) {
		return value, nil
	}
	if b == nil {
		return "", ErrNoMasterKey
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, sealedPrefix))
	if err != nil {
		return "", fmt.Errorf("secretbox: 密文不是合法 base64: %w", err)
	}
	nonceSize := b.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("%w: shorter than nonce", ErrBadCiphertext)
	}
	plain, err := b.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("secretbox: 解密失败（主密钥与加密时不一致？）: %w", err)
	}
	return string(plain), nil
}
