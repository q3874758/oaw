package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
)

// Wallet 钱包
type Wallet struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Private string `json:"private"`
	Public  string `json:"public"`
}

// NewWallet 创建新钱包
func NewWallet(name string) (*Wallet, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成私钥失败: %w", err)
	}

	pub := privateKey.PublicKey
	pubBytes := elliptic.Marshal(elliptic.P256(), pub.X, pub.Y)

	hash := sha256.Sum256(pubBytes)
	address := hex.EncodeToString(hash[:12])

	privateHex := fmt.Sprintf("%064x", privateKey.D)
	publicHex := hex.EncodeToString(pubBytes)

	return &Wallet{
		Name:    name,
		Address: address,
		Private: privateHex,
		Public:  publicHex,
	}, nil
}

// Save 保存钱包到文件
func (w *Wallet) Save(dir string) error {
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.json", w.Name))
	return os.WriteFile(filename, data, 0600)
}

// Load 从文件加载钱包
func Load(dir, name string) (*Wallet, error) {
	filename := filepath.Join(dir, fmt.Sprintf("%s.json", name))
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var w Wallet
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// Sign 签名消息
func (w *Wallet) Sign(message string) (string, error) {
	privateBytes, ok := new(big.Int).SetString(w.Private, 16)
	if !ok {
		return "", fmt.Errorf("解析私钥失败")
	}

	privateKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
		},
		D: privateBytes,
	}
	privateKey.PublicKey.X, privateKey.PublicKey.Y = privateKey.Curve.ScalarBaseMult(privateKey.D.Bytes())

	hash := sha256.Sum256([]byte(message))
	sig, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(sig), nil
}
