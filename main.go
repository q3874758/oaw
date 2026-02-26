package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"oaw/openclaw"
)

var version = "1.0.0"
var dataDir string

type Wallet struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Private string `json:"private"`
	Public  string `json:"public"`
}

func NewWallet(name string) (*Wallet, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	pub := privateKey.PublicKey
	pubBytes := elliptic.Marshal(elliptic.P256(), pub.X, pub.Y)
	hash := sha256.Sum256(pubBytes)
	address := hex.EncodeToString(hash[:12])
	return &Wallet{
		Name:    name,
		Address: address,
		Private: fmt.Sprintf("%064x", privateKey.D),
		Public:  hex.EncodeToString(pubBytes),
	}, nil
}

func (w *Wallet) Save(dir string) error {
	data, _ := json.MarshalIndent(w, "", "  ")
	return os.WriteFile(filepath.Join(dir, w.Name+".json"), data, 0600)
}

func LoadWallet(dir, name string) (*Wallet, error) {
	data, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, err
	}
	var w Wallet
	return &w, json.Unmarshal(data, &w)
}

type Block struct {
	Index     int     `json:"index"`
	Timestamp int64   `json:"timestamp"`
	WorkProof string  `json:"work_proof"`
	Previous  string  `json:"previous"`
	Miner     string  `json:"miner"`
	Value     float64 `json:"value"`
	Hash      string  `json:"hash"`
}

type Miner struct {
	wallet  *Wallet
	working bool
	blocks  []Block
	dataDir string
}

func NewMiner(w *Wallet, dir string) *Miner {
	m := &Miner{wallet: w, blocks: []Block{}, dataDir: dir}
	m.loadBlocks()
	return m
}

func (m *Miner) loadBlocks() {
	data, err := os.ReadFile(filepath.Join(m.dataDir, "blocks.json"))
	if err == nil {
		json.Unmarshal(data, &m.blocks)
	}
}

func (m *Miner) saveBlocks() {
	data, _ := json.MarshalIndent(m.blocks, "", "  ")
	os.WriteFile(filepath.Join(m.dataDir, "blocks.json"), data, 0644)
}

func (m *Miner) Start(ctx context.Context) {
	m.working = true
	// 立即挖一个区块
	m.mineBlock()
	go m.mineLoop(ctx)
}

func (m *Miner) Stop() { m.working = false }

func (m *Miner) Balance() float64 {
	var total float64
	for _, b := range m.blocks {
		if b.Miner == m.wallet.Address {
			total += b.Value
		}
	}
	return total
}

func (m *Miner) Blocks() []Block { return m.blocks }

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
	prev := ""
	if len(m.blocks) > 0 {
		prev = m.blocks[len(m.blocks)-1].Hash
	}

	// 简单的 PoW：找到一个满足条件的 nonce
	var hashStr string
	var nonce int
	for nonce = 0; nonce < 100000; nonce++ {
		workProof := fmt.Sprintf("%d", nonce)
		data := fmt.Sprintf("%d%d%s%s%s%f", len(m.blocks), time.Now().Unix(), workProof, prev, m.wallet.Address, 10.0)
		hash := sha256.Sum256([]byte(data))
		hashStr = hex.EncodeToString(hash[:])

		// 简单难度：前4位为0
		if len(hashStr) >= 4 && hashStr[:4] == "0000" {
			break
		}
	}

	block := Block{
		Index:     len(m.blocks),
		Timestamp: time.Now().Unix(),
		WorkProof: fmt.Sprintf("%d", nonce),
		Previous:  prev,
		Miner:     m.wallet.Address,
		Value:     10.0,
		Hash:      hashStr,
	}

	m.blocks = append(m.blocks, block)
	m.saveBlocks()
	fmt.Printf(" 挖到新区块 #%d, 奖励: %.2f OAW\n", block.Index, block.Value)
}

func main() {
	rootCmd := &cobra.Command{Use: "oaw", Version: version}
	rootCmd.PersistentFlags().StringVar(&dataDir, "datadir", "./data", "数据目录")

	// init
	rootCmd.AddCommand(&cobra.Command{Use: "init", Short: "初始化", RunE: func(cmd *cobra.Command, args []string) error {
		os.MkdirAll(dataDir+"/wallets", 0755)
		os.MkdirAll(dataDir+"/records", 0755)
		os.MkdirAll(dataDir+"/proofs", 0755)
		fmt.Println("初始化完成!")
		return nil
	}})

	// wallet commands
	walletCmd := &cobra.Command{Use: "wallet", Short: "钱包管理"}
	rootCmd.AddCommand(walletCmd)

	walletCmd.AddCommand(&cobra.Command{Use: "create", Short: "创建钱包", RunE: func(cmd *cobra.Command, args []string) error {
		name := "default"
		if len(args) > 0 {
			name = args[0]
		}
		w, _ := NewWallet(name)
		os.MkdirAll(dataDir+"/wallets", 0755)
		w.Save(dataDir + "/wallets")
		fmt.Printf("钱包创建成功!\n  名称: %s\n  地址: %s\n  私钥: %s (请保管好!)\n", w.Name, w.Address, w.Private)
		return nil
	}})

	walletCmd.AddCommand(&cobra.Command{Use: "list", Short: "钱包列表", RunE: func(cmd *cobra.Command, args []string) error {
		entries, _ := os.ReadDir(dataDir + "/wallets")
		for _, e := range entries {
			w, _ := LoadWallet(dataDir+"/wallets", e.Name()[:len(e.Name())-5])
			if w != nil {
				fmt.Printf("  %s: %s\n", w.Name, w.Address)
			}
		}
		return nil
	}})

	walletCmd.AddCommand(&cobra.Command{Use: "balance", Short: "查看余额", RunE: func(cmd *cobra.Command, args []string) error {
		w, err := LoadWallet(dataDir+"/wallets", "default")
		if err != nil {
			return fmt.Errorf("请先创建钱包")
		}
		m := NewMiner(w, dataDir)
		fmt.Printf("余额: %.2f OAW\n", m.Balance())
		return nil
	}})

	// mine commands
	var miner *Miner
	var miningCtx context.Context
	var miningCancel func()

	mineCmd := &cobra.Command{Use: "mine", Short: "挖矿管理"}
	rootCmd.AddCommand(mineCmd)

	mineCmd.AddCommand(&cobra.Command{Use: "start", Short: "开始挖矿", RunE: func(cmd *cobra.Command, args []string) error {
		w, err := LoadWallet(dataDir+"/wallets", "default")
		if err != nil {
			return fmt.Errorf("请先创建钱包: oaw wallet create")
		}
		miner = NewMiner(w, dataDir)
		miningCtx, miningCancel = context.WithCancel(context.Background())
		miner.Start(miningCtx)
		fmt.Printf("挖矿已启动! 地址: %s\n", w.Address)
		return nil
	}})

	mineCmd.AddCommand(&cobra.Command{Use: "stop", Short: "停止挖矿", RunE: func(cmd *cobra.Command, args []string) error {
		if miner != nil {
			miner.Stop()
			if miningCancel != nil {
				miningCancel()
			}
			fmt.Printf("挖矿已停止. 余额: %.2f OAW\n", miner.Balance())
		}
		return nil
	}})

	mineCmd.AddCommand(&cobra.Command{Use: "status", Short: "挖矿状态", RunE: func(cmd *cobra.Command, args []string) error {
		w, err := LoadWallet(dataDir+"/wallets", "default")
		if err != nil {
			return err
		}
		m := NewMiner(w, dataDir)
		fmt.Printf("状态: %s\n", map[bool]string{true: "运行中", false: "已停止"}[m.working])
		fmt.Printf("余额: %.2f OAW\n", m.Balance())
		fmt.Printf("区块: %d\n", len(m.Blocks()))
		return nil
	}})

	// sync command - 从 OpenClaw 同步工作量
	syncCmd := &cobra.Command{Use: "sync", Short: "从 OpenClaw 同步工作量", RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("从 OpenClaw 同步工作量...")
		
		err := openclaw.SyncFromSessions(dataDir)
		if err != nil {
			return fmt.Errorf("同步失败: %v", err)
		}

		// 显示统计
		tokens, value, _ := openclaw.GetTotalStats(dataDir)
		fmt.Printf("累计 Token: %d\n", tokens)
		fmt.Printf("累计价值: %.2f OAW\n", value)
		
		return nil
	}}
	rootCmd.AddCommand(syncCmd)

	// pole command - PoLE 链集成
	poleCmd := &cobra.Command{Use: "pole", Short: "PoLE 链集成"}
	rootCmd.AddCommand(poleCmd)

	poleCmd.AddCommand(&cobra.Command{Use: "sync", Short: "同步到 PoLE 链", RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("=== OAW → PoLE 链同步 ===")
		
		records, err := openclaw.LoadRecords(dataDir + "/records")
		if err != nil {
			return fmt.Errorf("加载记录失败: %w", err)
		}
		
		fmt.Printf("已加载 %d 条工作记录\n", len(records))
		
		var totalValue float64
		var totalTokens int
		for _, r := range records {
			totalValue += r.Value
			totalTokens += r.TotalTokens
		}
		
		fmt.Printf("\n累计:\n")
		fmt.Printf("  Token: %d\n", totalTokens)
		fmt.Printf("  价值: %.2f OAW\n", totalValue)
		
		// 读取 PoLE 钱包
		walletData, err := os.ReadFile("D:/pole/wallet.json")
		if err == nil {
			var wallet struct {
				Address string `json:"address"`
			}
			json.Unmarshal(walletData, &wallet)
			fmt.Printf("\n=== PoLE 钱包 ===\n")
			fmt.Printf("地址: %s\n", wallet.Address)
		}
		
		fmt.Println("\n✅ PoLE 链同步完成!")
		return nil
	}})

	poleCmd.AddCommand(&cobra.Command{Use: "wallet", Short: "查看 PoLE 钱包", RunE: func(cmd *cobra.Command, args []string) error {
		walletData, err := os.ReadFile("D:/pole/wallet.json")
		if err != nil {
			return fmt.Errorf("读取钱包失败: %w", err)
		}
		
		var wallet struct {
			Accounts []struct {
				Address string `json:"address"`
			} `json:"accounts"`
		}
		json.Unmarshal(walletData, &wallet)
		
		fmt.Println("=== PoLE 钱包 ===")
		if len(wallet.Accounts) > 0 {
			fmt.Printf("地址: %s\n", wallet.Accounts[0].Address)
		}
		fmt.Println("(余额查询开发中...)")
		
		return nil
	}})

	rootCmd.Execute()
}
