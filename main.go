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
	return &Miner{wallet: w, blocks: []Block{}, dataDir: dir}
}

func (m *Miner) Start(ctx context.Context) {
	m.working = true
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
	block := Block{
		Index:     len(m.blocks),
		Timestamp: time.Now().Unix(),
		WorkProof: fmt.Sprintf("%d", len(m.blocks)),
		Previous:  prev,
		Miner:     m.wallet.Address,
		Value:     10.0,
	}
	data := fmt.Sprintf("%d%d%s%s%s%f", block.Index, block.Timestamp, block.WorkProof, block.Previous, block.Miner, block.Value)
	hash := sha256.Sum256([]byte(data))
	block.Hash = hex.EncodeToString(hash[:])
	
	m.blocks = append(m.blocks, block)
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

	rootCmd.Execute()
}
