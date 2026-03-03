package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"

	"github.com/ethereum/go-ethereum/crypto"
)

// 加密配置
const (
	KeySize     = 32 // AES-256
	SaltSize    = 32
	Iterations  = 100000
	NonceSize   = 12
)

// WalletFile 钱包文件格式
type WalletFile struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	Encrypted bool   `json:"encrypted"` // 是否加密
	Cipher    string `json:"cipher"`   // 加密后的私钥 (hex)
	Salt      string `json:"salt"`     // 盐值 (hex)
	Public    string `json:"public"`
}

// Wallet 钱包 (使用 secp256k1 曲线，与 PoLE 链兼容)
type Wallet struct {
	Name         string `json:"-"`
	Address      string `json:"address"`      // 以太坊风格地址 (0x...)
	Private      string `json:"-"`            // 私钥 (不序列化)
	Public       string `json:"public"`       // 公钥
	PrivateKey   *ecdsa.PrivateKey `json:"-"` // 运行时使用，不序列化
}

// deriveKey 从密码派生密钥
func deriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, Iterations, KeySize, sha256.New)
}

// Encrypt 加密私钥
func (w *Wallet) Encrypt(password string) (cipherHex, saltHex string, err error) {
	if w.Private == "" {
		return "", "", fmt.Errorf("私钥为空")
	}

	// 生成盐值
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}

	// 派生密钥
	key := deriveKey(password, salt)

	// 创建 AES-GCM 加密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	// 加密
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(w.Private), nil)

	return hex.EncodeToString(ciphertext), hex.EncodeToString(salt), nil
}

// Decrypt 解密私钥
func (w *Wallet) Decrypt(password, cipherHex, saltHex string) error {
	ciphertext, err := hex.DecodeString(cipherHex)
	if err != nil {
		return err
	}

	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return err
	}

	// 派生密钥
	key := deriveKey(password, salt)

	// 创建 AES-GCM 解密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// 解密
	if len(ciphertext) < NonceSize {
		return fmt.Errorf("密文太短")
	}

	nonce, ciphertext := ciphertext[:NonceSize], ciphertext[NonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}

	w.Private = string(plaintext)

	// 恢复 PrivateKey
	w.PrivateKey, err = crypto.HexToECDSA(w.Private)
	return err
}

// NewWallet 创建新钱包 (使用 secp256k1)
func NewWallet(name string) (*Wallet, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("生成私钥失败: %w", err)
	}

	publicKey := privateKey.PublicKey
	
	// 生成以太坊风格地址
	address := crypto.PubkeyToAddress(publicKey)
	
	privateHex := crypto.FromECDSA(privateKey)
	publicHex := hex.EncodeToString(crypto.CompressPubkey(&publicKey))

	return &Wallet{
		Name:         name,
		Address:      address.Hex(),
		Private:      hex.EncodeToString(privateHex),
		Public:       publicHex,
		PrivateKey:   privateKey,
	}, nil
}

// NewWalletFromPrivateKey 从私钥创建钱包
func NewWalletFromPrivateKey(name, privateKeyHex string) (*Wallet, error) {
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败: %w", err)
	}

	publicKey := privateKey.PublicKey
	address := crypto.PubkeyToAddress(publicKey)
	publicHex := hex.EncodeToString(crypto.CompressPubkey(&publicKey))

	return &Wallet{
		Name:         name,
		Address:      address.Hex(),
		Private:      privateKeyHex,
		Public:       publicHex,
		PrivateKey:   privateKey,
	}, nil
}

// Save 保存钱包到文件 (明文)
func (w *Wallet) Save(dir string) error {
	wf := WalletFile{
		Name:    w.Name,
		Address: w.Address,
		Public:  w.Public,
	}
	
	data, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.json", w.Name))
	return os.WriteFile(filename, data, 0600)
}

// SaveEncrypted 保存加密钱包到文件
func (w *Wallet) SaveEncrypted(dir, password string) error {
	cipherHex, saltHex, err := w.Encrypt(password)
	if err != nil {
		return err
	}

	wf := WalletFile{
		Name:      w.Name,
		Address:   w.Address,
		Encrypted: true,
		Cipher:    cipherHex,
		Salt:      saltHex,
		Public:    w.Public,
	}

	data, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.json", w.Name))
	return os.WriteFile(filename, data, 0600)
}

// Load 从文件加载钱包 (自动检测是否加密)
func Load(dir, name string) (*Wallet, error) {
	filename := filepath.Join(dir, fmt.Sprintf("%s.json", name))
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var wf WalletFile
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	w := &Wallet{
		Name:    wf.Name,
		Address: wf.Address,
		Public:  wf.Public,
	}

	// 如果加密了，私钥需要解密
	if wf.Encrypted {
		// 返回带有占位符的钱包，调用者需要输入密码解密
		w.Private = ""
	} else {
		// 旧格式钱包可能直接存储私钥
		var oldWallet struct {
			Private string `json:"private"`
		}
		json.Unmarshal(data, &oldWallet)
		w.Private = oldWallet.Private
		if w.Private != "" {
			w.PrivateKey, _ = crypto.HexToECDSA(w.Private)
		}
	}

	return w, nil
}

// LoadAndDecrypt 加载并解密钱包
func LoadAndDecrypt(dir, name, password string) (*Wallet, error) {
	filename := filepath.Join(dir, fmt.Sprintf("%s.json", name))
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var wf WalletFile
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	w := &Wallet{
		Name:    wf.Name,
		Address: wf.Address,
		Public:  wf.Public,
	}

	if wf.Encrypted {
		if err := w.Decrypt(password, wf.Cipher, wf.Salt); err != nil {
			return nil, fmt.Errorf("密码错误: %w", err)
		}
	} else {
		// 尝试旧格式
		var oldWallet struct {
			Private string `json:"private"`
		}
		json.Unmarshal(data, &oldWallet)
		w.Private = oldWallet.Private
		if w.Private != "" {
			w.PrivateKey, _ = crypto.HexToECDSA(w.Private)
		}
	}

	return w, nil
}

// Sign 签名消息 (使用 secp256k1)
func (w *Wallet) Sign(message string) (string, error) {
	if w.PrivateKey == nil {
		var err error
		w.PrivateKey, err = crypto.HexToECDSA(w.Private)
		if err != nil {
			return "", fmt.Errorf("解析私钥失败: %w", err)
		}
	}

	sig, err := crypto.Sign([]byte(message), w.PrivateKey)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(sig), nil
}

// SignTx 签名交易
func (w *Wallet) SignTx(txData string) (string, error) {
	if w.PrivateKey == nil {
		var err error
		w.PrivateKey, err = crypto.HexToECDSA(w.Private)
		if err != nil {
			return "", fmt.Errorf("解析私钥失败: %w", err)
		}
	}

	sig, err := crypto.Sign([]byte(txData), w.PrivateKey)
	if err != nil {
		return "", err
	}

	return "0x" + hex.EncodeToString(sig), nil
}
