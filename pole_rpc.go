package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// PoLE RPC 客户端 (适配 PoLE REST API)
type PoleRPC struct {
	NodeURL string
}

// NewPoleRPC 创建 PoLE RPC 客户端
func NewPoleRPC(nodeURL string) *PoleRPC {
	return &PoleRPC{NodeURL: nodeURL}
}

// StatusResponse 状态响应
type StatusResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ChainID       string `json:"chain_id"`
		BlockHeight   int    `json:"block_height"`
		TotalAccounts int    `json:"total_accounts"`
	} `json:"data"`
}

// GetChainID 获取链 ID
func (p *PoleRPC) GetChainID() (string, error) {
	resp, err := p.doGet("/status")
	if err != nil {
		return "", err
	}

	var status StatusResponse
	if err := json.Unmarshal(resp, &status); err != nil {
		return "", err
	}

	return status.Data.ChainID, nil
}

// GetBlockNumber 获取区块高度
func (p *PoleRPC) GetBlockNumber() (string, error) {
	resp, err := p.doGet("/block/latest")
	if err != nil {
		return "", err
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Height int `json:"height"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}

	return fmt.Sprintf("0x%x", result.Data.Height), nil
}

// GetBalance 获取余额
func (p *PoleRPC) GetBalance(address string) (string, error) {
	resp, err := p.doGet("/account/balance?address=" + address)
	if err != nil {
		return "0", err
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Balance string `json:"balance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "0", err
	}

	return result.Data.Balance, nil
}

// GetTransactionCount 获取交易数量
func (p *PoleRPC) GetTransactionCount(address string) (string, error) {
	resp, err := p.doGet("/account/" + address)
	if err != nil {
		return "0", err
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Nonce uint64 `json:"nonce"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "0", err
	}

	return fmt.Sprintf("0x%x", result.Data.Nonce), nil
}

// SendTransaction 发送交易
func (p *PoleRPC) SendTransaction(from, to, data string) (string, error) {
	type TxRequest struct {
		From string `json:"from"`
		To   string `json:"to"`
		Data string `json:"data"`
	}

	req := TxRequest{
		From: from,
		To:   to,
		Data: data,
	}

	reqData, _ := json.Marshal(req)
	resp, err := p.doPost("/tx/broadcast", reqData)
	if err != nil {
		return "", err
	}

	var result struct {
		TxHash string `json:"tx_hash"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}

	return result.TxHash, nil
}

// SendSignedTransaction 发送已签名交易
func (p *PoleRPC) SendSignedTransaction(signedTx string) (string, error) {
	type BroadcastRequest struct {
		RawTx string `json:"raw_tx"`
	}

	req := BroadcastRequest{RawTx: signedTx}
	reqData, _ := json.Marshal(req)
	resp, err := p.doPost("/tx/broadcast", reqData)
	if err != nil {
		return "", err
	}

	var result struct {
		TxHash string `json:"tx_hash"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}

	return result.TxHash, nil
}

// GetTransactionByHash 根据哈希查询交易
func (p *PoleRPC) GetTransactionByHash(hash string) (map[string]interface{}, error) {
	resp, err := p.doGet("/tx/" + hash)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// CallContract 调用合约 view 方法 (PoLE 不支持，返回错误)
func (p *PoleRPC) CallContract(to, data string) (string, error) {
	return "", fmt.Errorf("PoLE 不支持 eth_call，请使用 REST API")
}

// doGet 发送 GET 请求
func (p *PoleRPC) doGet(path string) ([]byte, error) {
	url := p.NodeURL + path
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// doPost 发送 POST 请求
func (p *PoleRPC) doPost(path string, data []byte) ([]byte, error) {
	url := p.NodeURL + path
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// CreateWorkRecordTx 创建工作记录交易数据
func CreateWorkRecordTx(agentID string, tokens uint64) string {
	data := fmt.Sprintf("0x12345678%s%x", agentID, tokens)
	return data
}

// GenerateKeyPair 生成密钥对
func GenerateKeyPair() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// PrivateKeyToHex 私钥转十六进制
func PrivateKeyToHex(key *ecdsa.PrivateKey) string {
	return fmt.Sprintf("%064x", key.D.Bytes())
}

// PublicKeyToHex 公钥转十六进制
func PublicKeyToHex(key *ecdsa.PublicKey) string {
	return fmt.Sprintf("%04x%064x%064x", 0, key.X.Bytes(), key.Y.Bytes())
}

// SignData 签名数据
func SignData(data []byte, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	hash := sha256.Sum256(data)
	return ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
}

// SignTransaction 完整签名交易
func SignTransaction(txData, privateKeyHex string) (string, error) {
	if len(privateKeyHex) != 64 {
		return "", fmt.Errorf("私钥格式错误: 需要 64 位十六进制字符串")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("私钥解析失败: %w", err)
	}

	signature, err := crypto.Sign([]byte(txData), privateKey)
	if err != nil {
		return "", fmt.Errorf("签名失败: %w", err)
	}

	return "0x" + hex.EncodeToString(signature), nil
}

// SignTransactionWithChainID 使用 chainID 签名交易 (EIP-155)
func SignTransactionWithChainID(txData, privateKeyHex string, chainID uint64) (string, error) {
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("私钥解析失败: %w", err)
	}

	txHash := crypto.Keccak256Hash([]byte(txData))
	signature, err := crypto.Sign(txHash.Bytes(), privateKey)
	if err != nil {
		return "", fmt.Errorf("签名失败: %w", err)
	}

	signature[64] = byte(chainID) + 35 + (signature[64] - 35) % 2

	return "0x" + hex.EncodeToString(signature), nil
}
