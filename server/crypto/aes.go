// Package crypto 提供前后端报文 AES-256-CTR 加解密功能。
// 密钥优先从环境变量 AES_KEY 读取，未设置时使用默认值。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
)

// getAESKey 获取 AES 密钥，优先读取环境变量 AES_KEY，否则使用默认密钥。
func getAESKey() []byte {
	key := os.Getenv("AES_KEY")
	if key != "" {
		return []byte(key)
	}
	return []byte("0123456789abcdef0123456789abcdef")
}

// Encrypt 使用 AES-256-CTR 加密明文，返回 Base64 编码的密文。
// 密文格式：16字节随机IV + 加密数据，整体 Base64 编码。
func Encrypt(plaintext []byte) (string, error) {
	// 创建 AES-256 加密块
	block, err := aes.NewCipher(getAESKey())
	if err != nil {
		return "", err
	}

	// 分配密文缓冲区：IV(16字节) + 明文长度
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]

	// 生成随机 IV
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	// CTR 模式加密（流加密，无需填充）
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	// Base64 编码后返回
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 Base64 编码的密文，返回明文。
// 密文格式：Base64(16字节IV + 加密数据)。
func Decrypt(encoded string) ([]byte, error) {
	// Base64 解码
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	// 创建 AES-256 解密块
	block, err := aes.NewCipher(getAESKey())
	if err != nil {
		return nil, err
	}

	// 密文至少需要包含一个完整的 IV
	if len(ciphertext) < aes.BlockSize {
		return nil, errors.New("ciphertext too short")
	}

	// 分离 IV 和实际密文
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	// CTR 模式解密（与加密对称，XOR 操作可逆）
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)

	return ciphertext, nil
}