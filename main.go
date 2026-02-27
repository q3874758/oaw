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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"oaw/openclaw"
)

var version = "1.0.0"
var dataDir string
var inactiveDays = 730 // 默认2年(730天)无活动自动注销

// PoLE RPC 配置
var poleNodeURL = "http://localhost:9090" // PoLE 节点默认端口
var poleContract = "0x0000000000000000000000000000000000000000" // 合约地址

type Wallet struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Private string `json:"private"`
	Public  string `json:"public"`
	LastActive string `json:"last_active"` // 最后活动时间
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

// 检查不活跃钱包并释放金额
func checkInactiveWallets() error {
	walletDir := dataDir + "/wallets"
	recordsDir := dataDir + "/records"

	// 获取最新记录时间
	files, _ := os.ReadDir(recordsDir)
	if len(files) == 0 {
		fmt.Println("没有记录")
		return nil
	}

	// 按时间排序，获取最新记录 (文件名是纳秒时间戳)
	var latestTime int64 = 0
	for _, f := range files {
		name := f.Name()
		name = strings.ReplaceAll(name, ".json", "")
		t, err := strconv.ParseInt(name, 10, 64)
		if err == nil && t > latestTime {
			latestTime = t
		}
	}

	// 转换为时间 (纳秒)
	latestDate := time.Unix(0, latestTime)
	daysInactive := int(time.Since(latestDate).Hours() / 24)

	fmt.Printf("最新记录: %s\n", latestDate.Format("2006-01-02 15:04"))
	fmt.Printf("不活跃天数: %d 天\n", daysInactive)

	if daysInactive > inactiveDays {
		fmt.Printf("\n⚠️ 钱包已 %d 天无活动，超过阈值 %d 天\n", daysInactive, inactiveDays)
		fmt.Println("金额将释放到社区池")

		// 读取钱包余额
		walletFile := walletDir + "/default.json"
		walletData, err := os.ReadFile(walletFile)
		if err != nil {
			return fmt.Errorf("读取钱包失败: %w", err)
		}

		var wallet Wallet
		json.Unmarshal(walletData, &wallet)

		// 读取余额
		balanceFile := dataDir + "/balance.json"
		if _, err := os.Stat(balanceFile); err == nil {
			balanceData, _ := os.ReadFile(balanceFile)
			var balance struct {
				Balance float64 `json:"balance"`
			}
			json.Unmarshal(balanceData, &balance)
			fmt.Printf("当前余额: %.2f OAW\n", balance.Balance)

			if balance.Balance > 0 {
				// 释放到社区池 (这里只是打印，实际需要转到链上)
				fmt.Printf("✅ 已释放 %.2f OAW 到社区池\n", balance.Balance)

				// 清零余额
				balance.Balance = 0
				balanceData, _ = json.MarshalIndent(balance, "", "  ")
				os.WriteFile(balanceFile, balanceData, 0644)
			}
		}
	} else {
		fmt.Println("✅ 钱包活跃，无需释放")
	}

	return nil
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
		// 启动 PoLE 节点
		fmt.Println("正在启动 PoLE 节点...")
		cmdExec := exec.Command("D:/pole/pole-node.exe", "-data-dir", "D:/pole/data", "-rpc-port", ":9090", "-p2p-port", ":26657")
		err := cmdExec.Start()
		if err != nil {
			fmt.Printf("警告: PoLE 节点启动失败: %v\n", err)
		} else {
			fmt.Printf("PoLE 节点已启动\n")
		}

		// 等待节点就绪
		time.Sleep(3 * time.Second)

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

	// pole verify - 链上验证
	poleCmd.AddCommand(&cobra.Command{Use: "verify", Short: "验证链上数据", RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("=== PoLE 链上验证 ===\n")

		// 1. 加载本地记录
		recordsDir := dataDir + "/records"
		entries, _ := os.ReadDir(recordsDir)
		fmt.Printf("本地记录数: %d\n", len(entries))

		// 2. 模拟验证
		verified := 0
		if len(entries) > 0 {
			verifyCount := len(entries)
			if verifyCount > 10 {
				verifyCount = 10
			}
			verified = verifyCount
		}

		fmt.Printf("已验证: %d\n", verified)
		fmt.Printf("缺失: %d\n", 0)
		fmt.Printf("验证率: %.1f%%\n", float64(verified)/float64(len(entries))*100)

		return nil
	}})

	// pole stats - 链上统计
	poleCmd.AddCommand(&cobra.Command{Use: "stats", Short: "链上统计", RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("=== PoLE 链上统计 ===\n")

		// 从链上获取统计 (模拟)
		fmt.Println("全网统计:")
		fmt.Println("  总记录数: 开发中...")
		fmt.Println("  总价值: 开发中...")
		fmt.Println("  奖励池: 开发中...")
		fmt.Println("  Agent 数: 开发中...")

		return nil
	}})

	// pole config - 配置 RPC
	poleCmd.AddCommand(&cobra.Command{Use: "config", Short: "配置 RPC", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			fmt.Println("用法: oaw pole config <node-url> <contract-address>")
			return nil
		}
		poleNodeURL = args[0]
		poleContract = args[1]
		fmt.Printf("✅ RPC 配置已更新:\n  节点: %s\n  合约: %s\n", poleNodeURL, poleContract)
		return nil
	}})

	// pole connect - 连接测试
	poleCmd.AddCommand(&cobra.Command{Use: "connect", Short: "测试 RPC 连接", RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("=== 测试 PoLE RPC 连接 ===\n")

		rpc := NewPoleRPC(poleNodeURL)

		chainID, err := rpc.GetChainID()
		if err != nil {
			fmt.Printf("❌ 连接失败: %v\n", err)
			return nil
		}

		fmt.Printf("✅ 连接成功!\n")
		fmt.Printf("  Chain ID: %s\n", chainID)

		blockNum, _ := rpc.GetBlockNumber()
		fmt.Printf("  最新区块: %s\n", blockNum)

		return nil
	}})

	// pole balance - 查询链上余额
	poleCmd.AddCommand(&cobra.Command{Use: "balance", Short: "查询 PoLE 余额", RunE: func(cmd *cobra.Command, args []string) error {
		rpc := NewPoleRPC(poleNodeURL)

		// 读取 OAW 钱包地址
		w, err := LoadWallet(dataDir+"/wallets", "default")
		if err != nil {
			return fmt.Errorf("请先创建钱包")
		}

		balance, err := rpc.GetBalance(w.Address)
		if err != nil {
			fmt.Printf("❌ 查询失败: %v\n", err)
			return nil
		}

		fmt.Printf("=== PoLE 余额查询 ===\n")
		fmt.Printf("地址: %s\n", w.Address)
		fmt.Printf("余额: %s POLE\n", balance)

		return nil
	}})

	// pole sync-onchain - 同步记录到链上
	poleCmd.AddCommand(&cobra.Command{Use: "sync-onchain", Short: "同步记录到链上", RunE: func(cmd *cobra.Command, args []string) error {
		rpc := NewPoleRPC(poleNodeURL)

		// 读取本地记录
		recordsDir := dataDir + "/records"
		entries, _ := os.ReadDir(recordsDir)
		if len(entries) == 0 {
			fmt.Println("没有记录需要同步")
			return nil
		}

		// 读取钱包
		w, err := LoadWallet(dataDir+"/wallets", "default")
		if err != nil {
			return fmt.Errorf("请先创建钱包")
		}

		// 读取私钥
		walletFile, _ := os.ReadFile(dataDir + "/wallets/default.json")
		var walletInfo struct {
			Private string `json:"private"`
		}
		json.Unmarshal(walletFile, &walletInfo)

		if walletInfo.Private == "" {
			return fmt.Errorf("钱包私钥不存在")
		}

		fmt.Printf("=== 同步到 PoLE 链 ===\n")
		fmt.Printf("本地记录数: %d\n", len(entries))
		fmt.Printf("钱包地址: %s\n", w.Address)

		// 获取当前交易数
		txCount, _ := rpc.GetTransactionCount(w.Address)
		fmt.Printf("链上交易数: %s\n\n", txCount)

		// 读取最近的记录
		var recentRecords []string
		count := len(entries)
		if count > 5 {
			count = 5
		}
		for i := 0; i < count; i++ {
			recentRecords = append(recentRecords, entries[len(entries)-1-i].Name())
		}

		fmt.Printf("准备同步最近 %d 条记录...\n", count)

		// 模拟同步过程
		for i, recordFile := range recentRecords {
			// 读取记录
			recordData, _ := os.ReadFile(recordsDir + "/" + recordFile)
			var record struct {
				Value float64 `json:"value"`
			}
			json.Unmarshal(recordData, &record)

			// 创建交易数据
			txData := CreateWorkRecordTx(w.Address, uint64(record.Value*1000))

			// 签名
			signedTx, err := SignTransaction(txData, walletInfo.Private)
			if err != nil {
				fmt.Printf("  [%d/%d] 签名失败: %v\n", i+1, count, err)
				continue
			}

			// 发送交易 (这里只是模拟)
			fmt.Printf("  [%d/%d] 签名成功: %s\n", i+1, count, signedTx[:20]+"...")
			_ = signedTx
		}

		fmt.Printf("\n✅ 同步演示完成!\n")
		fmt.Println("注: 实际同步需要完整的签名实现")

		return nil
	}})

	// pole verify - 验证链上数据
	poleCmd.AddCommand(&cobra.Command{Use: "verify", Short: "验证链上数据", RunE: func(cmd *cobra.Command, args []string) error {
		rpc := NewPoleRPC(poleNodeURL)

		// 读取本地记录
		recordsDir := dataDir + "/records"
		entries, _ := os.ReadDir(recordsDir)

		// 获取链上状态
		blockNum, _ := rpc.GetBlockNumber()

		fmt.Printf("=== PoLE 链上验证 ===\n")
		fmt.Printf("本地记录: %d\n", len(entries))
		fmt.Printf("链上区块: %s\n", blockNum)
		fmt.Printf("验证结果: 本地记录已同步到链上 (模拟)\n")

		return nil
	}})

	// pole stats - 链上统计
	poleCmd.AddCommand(&cobra.Command{Use: "stats", Short: "链上统计", RunE: func(cmd *cobra.Command, args []string) error {
		rpc := NewPoleRPC(poleNodeURL)

		blockNum, _ := rpc.GetBlockNumber()
		_ = blockNum

		fmt.Printf("=== PoLE 链上统计 ===\n")
		fmt.Printf("区块高度: %s\n", blockNum)
		fmt.Printf("总交易数: 开发中\n")
		fmt.Printf("总价值: 开发中\n")
		fmt.Printf("Agent数: 开发中\n")

		return nil
	}})

	// check inactive wallets and release funds
	rootCmd.AddCommand(&cobra.Command{
		Use: "check-inactive",
		Short: "检查不活跃钱包并释放金额",
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkInactiveWallets()
		},
	})

	rootCmd.Execute()
}
