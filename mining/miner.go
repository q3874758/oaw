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

// 难度配置
const (
	MinDifficulty     = 2   // 最小难度
	MaxDifficulty     = 10  // 最大难度
	TargetBlockTime   = 10  // 目标区块时间 (秒)
	DifficultyAdjust  = 1   // 每次调整幅度
	BlockTimeWindow   = 10  // 计算区块时间的窗口大小
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
	wallet       *wallet.Wallet
	working      bool
	difficulty   int
	minDifficulty int
	maxDifficulty int
	blocks       []Block
	mu           sync.RWMutex
	dataDir      string
	lastBlockTime int64
}

// NewMiner 创建矿工
func NewMiner(w *wallet.Wallet, dataDir string) *Miner {
	return &Miner{
		wallet:       w,
		difficulty:   4, // 初始难度
		minDifficulty: MinDifficulty,
		maxDifficulty: MaxDifficulty,
		blocks:       []Block{},
		dataDir:      dataDir,
		lastBlockTime: time.Now().Unix(),
	}
}

// SetDifficultyRange 设置难度范围
func (m *Miner) SetDifficultyRange(min, max int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.minDifficulty = min
	m.maxDifficulty = max
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
		if nonce > 1000000 {
			break
		}
	}

	m.blocks = append(m.blocks, block)
	
	// 动态调整难度
	m.adjustDifficulty()
	
	// 保存到文件
	m.saveBlocks()
	
	fmt.Printf("✅ 挖到新区块 #%d, 奖励: %.2f OAW (难度: %d)\n", block.Index, block.Value, m.difficulty)
}

// adjustDifficulty 动态调整难度
func (m *Miner) adjustDifficulty() {
	if len(m.blocks) < 2 {
		return
	}

	// 计算最近几个区块的平均生成时间
	window := BlockTimeWindow
	if window > len(m.blocks) {
		window = len(m.blocks)
	}
	
	var totalTime int64
	for i := len(m.blocks) - window; i < len(m.blocks)-1; i++ {
		totalTime += m.blocks[i+1].Timestamp - m.blocks[i].Timestamp
	}
	avgBlockTime := totalTime / int64(window-1)
	
	// 根据平均区块时间调整难度
	if avgBlockTime < TargetBlockTime/2 {
		// 生成太快，增加难度
		m.difficulty += DifficultyAdjust
	} else if avgBlockTime > TargetBlockTime*2 {
		// 生成太慢，降低难度
		m.difficulty -= DifficultyAdjust
	}
	
	// 限制难度范围
	if m.difficulty < m.minDifficulty {
		m.difficulty = m.minDifficulty
	}
	if m.difficulty > m.maxDifficulty {
		m.difficulty = m.maxDifficulty
	}
}

// GetDifficulty 获取当前难度
func (m *Miner) GetDifficulty() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.difficulty
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
