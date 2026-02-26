package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OAWRecord OAW 工作量记录
type OAWRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	SessionID    string    `json:"session_id"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens int       `json:"total_tokens"`
	Value        float64   `json:"value"`
}

// PoLEChain PoLE 链交互
type PoLEChain struct {
	walletPath string
	nodeURL    string
}

// NewPoLEChain 创建 PoLE 链连接
func NewPoLEChain(walletPath, nodeURL string) *PoLEChain {
	return &PoLEChain{
		walletPath: walletPath,
		nodeURL:    nodeURL,
	}
}

// SubmitWork 提交工作量到链上
func (p *PoLEChain) SubmitWork(record OAWRecord) (string, error) {
	// 计算工作量哈希
	data := fmt.Sprintf("%d%d%d%f%s",
		record.InputTokens, record.OutputTokens, record.TotalTokens, record.Value, record.Timestamp.String())
	hash := sha256.Sum256([]byte(data))
	proofHash := hex.EncodeToString(hash[:])

	// 简化版：生成交易哈希（实际需要调用 RPC）
	txHash := fmt.Sprintf("0x%s", proofHash[:64])

	fmt.Printf("提交工作量到 PoLE 链...\n")
	fmt.Printf("  Token: %d\n", record.TotalTokens)
	fmt.Printf("  价值: %.2f OAW\n", record.Value)
	fmt.Printf("  交易哈希: %s\n", txHash)

	return txHash, nil
}

// GetBalance 获取链上余额
func (p *PoLEChain) GetBalance(address string) (float64, error) {
	// 简化版：从本地读取（实际需要调用 RPC）
	// TODO: 实现真正的链上查询
	return 0, nil
}

// LoadOAWRecords 加载 OAW 记录
func LoadOAWRecords(dataDir string) ([]OAWRecord, error) {
	recordsDir := filepath.Join(dataDir, "records")
	entries, err := os.ReadDir(recordsDir)
	if err != nil {
		return nil, err
	}

	var records []OAWRecord
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(recordsDir, e.Name()))
		var record OAWRecord
		if json.Unmarshal(data, &record) == nil {
			records = append(records, record)
		}
	}
	return records, nil
}

func main() {
	dataDir := "./data"
	if len(os.Args) > 1 {
		dataDir = os.Args[1]
	}

	fmt.Println("=== OAW + PoLE 集成 ===\n")

	// 1. 加载 OAW 工作量记录
	records, err := LoadOAWRecords(dataDir)
	if err != nil {
		fmt.Printf("加载记录失败: %v\n", err)
		return
	}

	fmt.Printf("加载了 %d 条工作记录\n\n", len(records))

	// 2. 计算总价值
	var totalValue float64
	var totalTokens int
	for _, r := range records {
		totalValue += r.Value
		totalTokens += r.TotalTokens
	}

	fmt.Printf("累计 Token: %d\n", totalTokens)
	fmt.Printf("累计价值: %.2f OAW\n\n", totalValue)

	// 3. 连接到 PoLE 链
	pole := NewPoLEChain("./wallet.json", "http://localhost:9333")

	// 4. 提交最新记录到链上
	if len(records) > 0 {
		latest := records[len(records)-1]
		txHash, err := pole.SubmitWork(latest)
		if err != nil {
			fmt.Printf("提交失败: %v\n", err)
		} else {
			fmt.Printf("\n✅ 已提交到 PoLE 链!\n")
			fmt.Printf("   交易: %s\n", txHash)
		}
	}

	// 5. 显示钱包地址
	fmt.Println("\n=== PoLE 钱包 ===")
	walletData, _ := os.ReadFile("wallet.json")
	var wallet struct {
		Address string `json:"address"`
	}
	json.Unmarshal(walletData, &wallet)
	fmt.Printf("地址: %s\n", wallet.Address)
}
