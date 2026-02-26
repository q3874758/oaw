package mining

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"oaw/wallet"
)

// Block 区块
type Block struct {
	Index        int       `json:"index"`
	Timestamp    int64     `json:"timestamp"`
	WorkProof   string    `json:"work_proof"`
	PreviousHash string    `json:"previous_hash"`
	Miner        string    `json:"miner"`
	Value        float64   `json:"value"`
	Hash         string    `json:"hash"`
}

// Miner 矿工
type Miner struct {
	wallet     *wallet.Wallet
	working    bool
	difficulty int
	blocks     []Block
	mu         sync.RWMutex
	dataDir    string
}

// NewMiner 创建矿工
func NewMiner(w *wallet.Wallet, dataDir string) *Miner {
	return &Miner{
		wallet:     w,
		difficulty: 4, // 初始难度
		blocks:     []Block{},
		dataDir:    dataDir,
	}
}

// Start 开始挖矿
func (m *Miner) Start(ctx context.Context) {
	m.working = true
	go m.mineLoop(ctx)
}

// Stop 停止挖矿
func (m *Miner) Stop() {
	m.working = false
}

// IsWorking 是否在挖矿
func (m *Miner) IsWorking() bool {
	return m.working
}

// GetBalance 获取余额
func (m *Miner) GetBalance() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var total float64
	for _, b := range m.blocks {
		if b.Miner == m.wallet.Address {
			total += b.Value
		}
	}
	return total
}

// GetBlocks 获取区块
func (m *Miner) GetBlocks() []Block {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	blocks := make([]Block, len(m.blocks))
	copy(blocks, m.blocks)
	return blocks
}

func (m *Miner) mineLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.working {
				m.mineBlock()
			}
		}
	}
}

func (m *Miner) mineBlock() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 创建新区块
	prevHash := ""
	if len(m.blocks) > 0 {
		prevHash = m.blocks[len(m.blocks)-1].Hash
	}

	block := Block{
		Index:        len(m.blocks),
		Timestamp:    time.Now().Unix(),
		PreviousHash: prevHash,
		Miner:        m.wallet.Address,
		Value:        10.0, // 挖矿奖励
	}

	// 工作量证明 (简化版)
	nonce := 0
	for {
		block.WorkProof = fmt.Sprintf("%d", nonce)
		block.Hash = m.calculateHash(block)
		
		if m.isValidProof(block) {
			break
		}
		nonce++
		
		// 防止无限循环
		if nonce > 100000 {
			break
		}
	}

	m.blocks = append(m.blocks, block)
	
	// 保存到文件
	m.saveBlocks()
	
	fmt.Printf("✅ 挖到新区块 #%d, 奖励: %.2f OAW\n", block.Index, block.Value)
}

func (m *Miner) calculateHash(b Block) string {
	data := fmt.Sprintf("%d%d%s%s%s%f",
		b.Index, b.Timestamp, b.WorkProof, b.PreviousHash, b.Miner, b.Value)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (m *Miner) isValidProof(b Block) bool {
	// 简化的难度检查
	hash := b.Hash
	for i := 0; i < m.difficulty; i++ {
		if i >= len(hash) || hash[i] != '0' {
			return false
		}
	}
	return true
}

func (m *Miner) saveBlocks() {
	data, _ := json.MarshalIndent(m.blocks, "", "  ")
	filename := filepath.Join(m.dataDir, "blocks.json")
	os.WriteFile(filename, data, 0644)
}

// LoadBlocks 加载区块
func (m *Miner) LoadBlocks() error {
	filename := filepath.Join(m.dataDir, "blocks.json")
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &m.blocks)
}
