package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
)

// PoLE RPC 客户端 (简化版)
type PoleRPC struct {
	NodeURL string
}

// NewPoleRPC 创建 PoLE RPC 客户端
func NewPoleRPC(nodeURL string) *PoleRPC {
	return &PoleRPC{NodeURL: nodeURL}
}

// RPCRequest RPC 请求
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string       `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int          `json:"id"`
}

// RPCResponse RPC 响应
type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID     int          `json:"id"`
	Result interface{}  `json:"result,omitempty"`
	Error  *RPCError   `json:"error,omitempty"`
}

// RPCError RPC 错误
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Call 调用 RPC 方法
func (p *PoleRPC) Call(method string, params ...interface{}) (*RPCResponse, error) {
	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	data, _ := json.Marshal(req)
	resp, err := http.Post(p.NodeURL, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result RPCResponse
	json.Unmarshal(body, &result)

	if result.Error != nil {
		return nil, fmt.Errorf("RPC错误: %s", result.Error.Message)
	}

	return &result, nil
}

// GetChainID 获取链 ID
func (p *PoleRPC) GetChainID() (string, error) {
	resp, err := p.Call("eth_chainId")
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Result == nil {
		return "unknown", nil
	}
	switch v := resp.Result.(type) {
	case string:
		return v, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// GetBalance 获取余额
func (p *PoleRPC) GetBalance(address string) (string, error) {
	resp, err := p.Call("eth_getBalance", address, "latest")
	if err != nil {
		return "0", err
	}
	if resp == nil || resp.Result == nil {
		return "0", nil
	}
	switch v := resp.Result.(type) {
	case string:
		return v, nil
	default:
		return "0", nil
	}
}

// GetBlockNumber 获取最新区块
func (p *PoleRPC) GetBlockNumber() (string, error) {
	resp, err := p.Call("eth_blockNumber")
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Result == nil {
		return "0", nil
	}
	switch v := resp.Result.(type) {
	case string:
		return v, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// CallContract 调用合约 view 方法
func (p *PoleRPC) CallContract(to, data string) (string, error) {
	resp, err := p.Call("eth_call", map[string]string{
		"to":   to,
		"data": data,
	}, "latest")
	if err != nil {
		return "", err
	}
	return resp.Result.(string), nil
}

// SendTransaction 发送交易
func (p *PoleRPC) SendTransaction(from, to, data string) (string, error) {
	resp, err := p.Call("eth_sendTransaction", map[string]string{
		"from": from,
		"to":   to,
		"data": data,
	})
	if err != nil {
		return "", err
	}
	return resp.Result.(string), nil
}

// GetTransactionCount 获取交易数量
func (p *PoleRPC) GetTransactionCount(address string) (string, error) {
	resp, err := p.Call("eth_getTransactionCount", address, "latest")
	if err != nil {
		return "0", err
	}
	if resp == nil || resp.Result == nil {
		return "0", nil
	}
	switch v := resp.Result.(type) {
	case string:
		return v, nil
	default:
		return "0", nil
	}
}

// SendSignedTransaction 发送已签名交易
func (p *PoleRPC) SendSignedTransaction(signedTx string) (string, error) {
	resp, err := p.Call("eth_sendRawTransaction", signedTx)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Result == nil {
		return "", fmt.Errorf("no result")
	}
	switch v := resp.Result.(type) {
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("unexpected result type")
	}
}

// GetTransactionByHash 根据哈希查询交易
func (p *PoleRPC) GetTransactionByHash(hash string) (map[string]interface{}, error) {
	resp, err := p.Call("eth_getTransactionByHash", hash)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Result == nil {
		return nil, nil
	}
	switch v := resp.Result.(type) {
	case map[string]interface{}:
		return v, nil
	default:
		return nil, nil
	}
}

// ParseTxResult 解析交易结果
func ParseTxResult(resp *RPCResponse) (string, error) {
	if resp == nil || resp.Result == nil {
		return "", fmt.Errorf("no result")
	}
	switch v := resp.Result.(type) {
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("unexpected type")
	}
}

// CreateWorkRecordTx 创建工作记录交易数据
func CreateWorkRecordTx(agentID string, tokens uint64) string {
	// 简化版 - 实际需要根据合约 ABI 编码
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
	// 解析私钥
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("私钥格式错误: %w", err)
	}

	// 生成密钥对（从私钥派生）
	privateKey := &ecdsa.PrivateKey{
		D: new(big.Int).SetBytes(privateKeyBytes),
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     elliptic.P256().Params().Gx,
			Y:     elliptic.P256().Params().Gy,
		},
	}

	// 签名交易数据
	signature, err := SignData([]byte(txData), privateKey)
	if err != nil {
		return "", fmt.Errorf("签名失败: %w", err)
	}

	// 编码为 RLP + 签名 (简化版)
	return "0x" + hex.EncodeToString(signature), nil
}
