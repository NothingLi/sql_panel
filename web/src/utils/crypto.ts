// AES-256-CTR 加解密工具，使用 crypto-js 实现（兼容 HTTP 和 HTTPS）。
// 密钥优先从 VITE_AES_KEY 环境变量读取（Vite 构建时注入），未设置时使用默认值。
// 与后端 server/crypto/aes.go 的加密算法和密钥必须保持一致。

import CryptoJS from 'crypto-js'

const AES_KEY = import.meta.env.VITE_AES_KEY || '0123456789abcdef0123456789abcdef'

// wordArrayToUint8Array 将 CryptoJS WordArray 转换为 Uint8Array
function wordArrayToUint8Array(wordArray: CryptoJS.lib.WordArray): Uint8Array {
  const arrayOfWords = wordArray.hasOwnProperty('words') ? wordArray.words : []
  const length = wordArray.hasOwnProperty('sigBytes') ? wordArray.sigBytes : arrayOfWords.length * 4
  const uInt8Array = new Uint8Array(length)
  let index = 0
  for (let i = 0; i < arrayOfWords.length; i++) {
    const word = arrayOfWords[i]
    uInt8Array[index++] = (word >> 24) & 0xff
    uInt8Array[index++] = (word >> 16) & 0xff
    uInt8Array[index++] = (word >> 8) & 0xff
    uInt8Array[index++] = word & 0xff
  }
  return uInt8Array.slice(0, length)
}

// uint8ArrayToWordArray 将 Uint8Array 转换为 CryptoJS WordArray
function uint8ArrayToWordArray(uInt8Array: Uint8Array): CryptoJS.lib.WordArray {
  const length = uInt8Array.length
  const words: number[] = []
  for (let i = 0; i < length; i += 4) {
    words.push(
      (uInt8Array[i] << 24) |
      ((uInt8Array[i + 1] || 0) << 16) |
      ((uInt8Array[i + 2] || 0) << 8) |
      (uInt8Array[i + 3] || 0)
    )
  }
  return CryptoJS.lib.WordArray.create(words, length)
}

// getKey 确保密钥长度为 32 字节（256位）
function getKey(): string {
  let key = AES_KEY
  if (key.length < 32) {
    key = key.padEnd(32, '0')
  } else if (key.length > 32) {
    key = key.substring(0, 32)
  }
  return key
}

// generateIV 生成 16 字节随机 IV
function generateIV(): CryptoJS.lib.WordArray {
  return CryptoJS.lib.WordArray.random(16)
}

// encrypt 加密明文字符串，返回 Base64 编码的密文。
// 密文格式：16字节随机IV + AES-CTR加密数据，整体 Base64 编码。
// 与 Go 后端 server/crypto/aes.go 格式完全兼容。
export async function encrypt(plaintext: string): Promise<string> {
  const key = getKey()
  const iv = generateIV()
  
  // 使用 AES-CTR 模式加密
  const encrypted = CryptoJS.AES.encrypt(
    plaintext,
    CryptoJS.enc.Utf8.parse(key),
    {
      iv: iv,
      mode: CryptoJS.mode.CTR,
      padding: CryptoJS.pad.NoPadding
    }
  )
  
  // 将 IV 和密文转换为 Uint8Array 并拼接
  const ivBytes = wordArrayToUint8Array(iv)
  const ciphertextBytes = wordArrayToUint8Array(encrypted.ciphertext)
  
  const combined = new Uint8Array(ivBytes.length + ciphertextBytes.length)
  combined.set(ivBytes)
  combined.set(ciphertextBytes, ivBytes.length)
  
  // Base64 编码
  return btoa(String.fromCharCode(...combined))
}

// decrypt 解密 Base64 编码的密文，返回明文字符串。
// 密文格式：Base64(16字节IV + AES-CTR加密数据)。
// 与 Go 后端 server/crypto/aes.go 格式完全兼容。
export async function decrypt(encoded: string): Promise<string> {
  const key = getKey()
  
  // Base64 解码
  const binary = atob(encoded)
  const combined = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    combined[i] = binary.charCodeAt(i)
  }
  
  // 分离 IV（前 16 字节）和密文
  const ivBytes = combined.slice(0, 16)
  const ciphertextBytes = combined.slice(16)
  
  const iv = uint8ArrayToWordArray(ivBytes)
  const ciphertext = uint8ArrayToWordArray(ciphertextBytes)
  
  // 使用 AES-CTR 模式解密
  const decrypted = CryptoJS.AES.decrypt(
    { ciphertext: ciphertext },
    CryptoJS.enc.Utf8.parse(key),
    {
      iv: iv,
      mode: CryptoJS.mode.CTR,
      padding: CryptoJS.pad.NoPadding
    }
  )
  
  return decrypted.toString(CryptoJS.enc.Utf8)
}

// canEncrypt 检查当前环境是否支持加密功能
export function canEncrypt(): boolean {
  return typeof CryptoJS !== 'undefined'
}
