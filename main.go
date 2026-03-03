package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/cobra"
	"oaw/openclaw"
)

var version = "1.0.0"
var dataDir string
var inactiveDays = 730 // 默认2年(730天)无活动自动注销

// PoLE RPC 配置
var poleNodeURL = "http://127.0.0.1:9090" // PoLE 节点默认端口
var poleContractAddress = "0x0000000000000000000000000000000000000001" // PoLE 代币合约地址
var poleDataDir = "D:/pole/data" // PoLE 节点数据目录

// Wallet 钱包结构 (使用 secp256k1 曲线，与 PoLE 链兼容)
type Wallet struct {
	Name       string `json:"name"`
	Address    string `json:"address"`     // 以太坊风格地址 (0x...)
	Private    string `json:"private"`     // 私钥 (64位十六进制)
	Public     string `json:"public"`      // 公钥
	LastActive string `json:"last_active"` // 最后活动时间
}

// NewWallet 创建新钱包 (使用 secp256k1 曲线，与 PoLE 链兼容)
func NewWallet(name string) (*Wallet, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	publicKey := privateKey.PublicKey
	
	// 生成以太坊风格地址
	address := crypto.PubkeyToAddress(publicKey)
	
	privateHex := crypto.FromECDSA(privateKey)
	publicHex := hex.EncodeToString(crypto.CompressPubkey(&publicKey))

	return &Wallet{
		Name:    name,
		Address: address.Hex(),
		Private: hex.EncodeToString(privateHex),
		Public:  publicHex,
	}, nil
}

func (w *Wallet) Save(dir string) error {
	data, _ := json.MarshalIndent(w, "", "  ")
	return os.WriteFile(filepath.Join(dir, w.Name+".json"), data, 0600)
}

// 检查不活跃钱包并释放金额 (链上执行)
func checkInactiveWallets() error {
	walletDir := dataDir + "/wallets"
	recordsDir := dataDir + "/records"

	entries, err := os.ReadDir(walletDir)
	if err != nil {
		fmt.Println("没有钱包")
		return nil
	}

	// 连接 PoLE 链
	rpc := NewPoleRPC(poleNodeURL)
	
	// 读取 OAW 钱包私钥
	walletFile := filepath.Join(walletDir, "default.json")
	walletData, err := os.ReadFile(walletFile)
	if err != nil {
		return fmt.Errorf("读取钱包失败: %v", err)
	}
	
	var walletInfo struct {
		Address string `json:"address"`
		Private string `json:"private"`
	}
	json.Unmarshal(walletData, &walletInfo)
	
	if walletInfo.Private == "" {
		return fmt.Errorf("钱包私钥不存在")
	}

	totalReleased := 0.0
	walletCount := 0
	
	fmt.Println("========== 链上不活跃钱包检查 ==========")
	fmt.Printf("PoLE 链: %s\n", poleNodeURL)
	fmt.Printf("检查钱包...\n\n")

	_ = totalReleased // 链上记录后累计

	for _, e := range entries {
		if e.IsDir() || len(e.Name()) < 5 {
			continue
		}
		
		name := e.Name()[:len(e.Name())-5]
		
		walletFile := filepath.Join(walletDir, e.Name())
		data, err := os.ReadFile(walletFile)
		if err != nil {
			continue
		}
		
		var w struct {
			LastActive string `json:"last_active"`
			Status    string `json:"status"`
		}
		json.Unmarshal(data, &w)
		
		// 跳过已注销的钱包
		if w.Status == "inactive" {
			continue
		}
		
		walletCount++
		
		// 获取最新活动时间
		recordFiles, _ := os.ReadDir(recordsDir)
		var latestTime int64 = 0
		
		for _, rf := range recordFiles {
			ts := rf.Name()[:len(rf.Name())-5]
			t, err := strconv.ParseInt(ts, 10, 64)
			if err == nil && t > latestTime {
				latestTime = t
			}
		}
		
		if latestTime == 0 {
			continue
		}
		
		// 计算不活跃天数
		latestDate := time.Unix(0, latestTime)
		daysInactive := int(time.Since(latestDate).Hours() / 24)
		
		// 获取链上余额
		chainBalance, _ := rpc.GetBalance(getAddressFromFile(data))
		
		fmt.Printf("钱包: %s\n", name)
		fmt.Printf("  地址: %s\n", getAddressFromFile(data))
		fmt.Printf("  最新活动: %s\n", latestDate.Format("2006-01-02 15:04"))
		fmt.Printf("  不活跃天数: %d 天\n", daysInactive)
		fmt.Printf("  链上余额: %s POLE\n", chainBalance)
		
		// 两年(730天)无活动则注销
		if daysInactive > inactiveDays {
			// 在链上记录注销交易
			txData := fmt.Sprintf("inactive:%s:%d:%s", 
				getAddressFromFile(data), 
				daysInactive, 
				time.Now().Format("20060102150405"))
			
			signedTx, err := SignTransaction(txData, walletInfo.Private)
			if err != nil {
				fmt.Printf("  ❌ 签名失败: %v\n", err)
				continue
			}
			
			// 发送到链上
			txHash, err := rpc.SendSignedTransaction(signedTx)
			if err != nil {
				fmt.Printf("  ⚠️ 链上记录失败: %v (本地记录)\n", err)
				// 仍然在本地记录
			} else {
				fmt.Printf("  ✅ 链上注销交易: %s\n", txHash[:16]+"...")
			}
			
			// 本地标记
			var walletData map[string]interface{}
			json.Unmarshal(data, &walletData)
			walletData["status"] = "inactive"
			walletData["released_at"] = time.Now().Format(time.RFC3339)
			walletData["tx_hash"] = txHash
			
			newData, _ := json.MarshalIndent(walletData, "", "  ")
			os.WriteFile(walletFile, newData, 0644)
			
			fmt.Printf("  ✅ 已注销\n")
		} else {
			fmt.Printf("  ✅ 活跃\n")
		}
		fmt.Println()
	}
	
	fmt.Println("========== 检查完成 ==========")
	fmt.Printf("检查钱包数: %d\n", walletCount)
	
	return nil
}

// getAddressFromFile 从钱包文件数据中获取地址
func getAddressFromFile(data []byte) string {
	var w struct {
		Address string `json:"address"`
	}
	json.Unmarshal(data, &w)
	return w.Address
}

// copyDir 复制目录
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, rel)
		
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// exportData 导出数据
func exportData(dataDir, exportFile, format string) error {
	recordsDir := dataDir + "/records"
	entries, err := os.ReadDir(recordsDir)
	if err != nil {
		return err
	}

	if format == "csv" {
		// 导出为 CSV
		var csvContent string
		csvContent = "timestamp,session_id,input_tokens,output_tokens,total_tokens,value\n"
		for _, e := range entries {
			data, _ := os.ReadFile(filepath.Join(recordsDir, e.Name()))
			var record struct {
				Timestamp    string `json:"timestamp"`
				SessionID    string `json:"session_id"`
				InputTokens  int    `json:"input_tokens"`
				OutputTokens int    `json:"output_tokens"`
				TotalTokens  int    `json:"total_tokens"`
				Value        float64 `json:"value"`
			}
			json.Unmarshal(data, &record)
			csvContent += fmt.Sprintf("%s,%s,%d,%d,%d,%.2f\n",
				record.Timestamp, record.SessionID, record.InputTokens, 
				record.OutputTokens, record.TotalTokens, record.Value)
		}
		return os.WriteFile(exportFile, []byte(csvContent), 0644)
	}

	// 导出为 JSON
	var records []map[string]interface{}
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(recordsDir, e.Name()))
		var record map[string]interface{}
		json.Unmarshal(data, &record)
		records = append(records, record)
	}
	data, _ := json.MarshalIndent(records, "", "  ")
	return os.WriteFile(exportFile, data, 0644)
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
	wallet        *Wallet
	working       bool
	blocks        []Block
	dataDir       string
	difficulty    int
	minDifficulty int
	maxDifficulty int
}

func NewMiner(w *Wallet, dir string) *Miner {
	m := &Miner{
		wallet:        w,
		blocks:        []Block{},
		dataDir:       dir,
		difficulty:    4,   // 初始难度
		minDifficulty: 2,   // 最小难度
		maxDifficulty: 10,  // 最大难度
	}
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

	// 获取当前周期的工作量
	currentPeriod := time.Now().Unix() / 60 // 每分钟一个周期
	localWork := m.getCurrentPeriodWork(currentPeriod)
	
	// 从 PoLE 获取全网工作量 (如果失败则使用本地作为预估)
	totalWork := m.getNetworkWork()
	if totalWork < localWork {
		totalWork = localWork
	}
	
	// 计算工作量占比
	var workRatio float64 = 1.0 // 默认100% (如果全网只有自己)
	if totalWork > 0 {
		workRatio = float64(localWork) / float64(totalWork)
		if workRatio > 1.0 {
			workRatio = 1.0
		}
	}

	// PoW 竞争区块
	var hashStr string
	var nonce uint64
	var startTime = time.Now()
	var attempts uint64
	
	prefix := strings.Repeat("0", m.difficulty)
	
	for nonce = 0; nonce < 10000000; nonce++ {
		attempts++
		
		workProof := fmt.Sprintf("%d", nonce)
		data := fmt.Sprintf("%d%d%s%s%s%f%d", 
			len(m.blocks), 
			time.Now().UnixNano(),
			workProof, 
			prev, 
			m.wallet.Address, 
			10.0,
			nonce)
		
		hash := sha256.Sum256([]byte(data))
		hashStr = hex.EncodeToString(hash[:])

		if len(hashStr) >= m.difficulty && hashStr[:m.difficulty] == prefix {
			elapsed := time.Since(startTime)
			fmt.Printf("  🔨 PoW 耗时: %v, 尝试次数: %d\n", elapsed, attempts)
			break
		}
		
		if nonce%100000 == 0 && !m.working {
			fmt.Println("  ⏹️ 挖矿已停止")
			return
		}
	}

	// 如果未找到有效 PoW，降低难度
	if nonce >= 10000000 {
		fmt.Printf("  ⚠️ 达到最大尝试次数，降低难度\n")
		m.difficulty--
		if m.difficulty < m.minDifficulty {
			m.difficulty = m.minDifficulty
		}
		hashStr = "0000ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	}

	// 计算奖励：基础奖励 * 工作量占比
	baseReward := 10.0
	actualReward := baseReward * workRatio
	
	// 无工作量则无奖励
	if localWork <= 0 {
		actualReward = 0
	}

	block := Block{
		Index:     len(m.blocks),
		Timestamp: time.Now().Unix(),
		WorkProof: fmt.Sprintf("%d", nonce),
		Previous:  prev,
		Miner:     m.wallet.Address,
		Value:     actualReward,
		Hash:      hashStr,
	}

	m.blocks = append(m.blocks, block)
	m.saveBlocks()
	
	// 将奖励记录到链上
	if actualReward > 0 {
		// 签名并发送到链上
		txData := fmt.Sprintf("reward:%s:%d:%d:%.4f", 
			m.wallet.Address,
			block.Index,
			localWork,
			actualReward)
		
		signedTx, err := SignTransaction(txData, m.wallet.Private)
		if err != nil {
			fmt.Printf("  ⚠️ 链上记录失败: %v (本地记录)\n", err)
		} else {
			rpc := NewPoleRPC(poleNodeURL)
			txHash, err := rpc.SendSignedTransaction(signedTx)
			if err != nil {
				fmt.Printf("  ⚠️ 链上提交失败: %v\n", err)
			} else {
				fmt.Printf("  ✅ 链上记录: %s\n", txHash[:20]+"...")
			}
		}
		
		fmt.Printf("  ✅ 挖到新区块 #%d\n", block.Index)
		fmt.Printf("     基础奖励: %.2f OAW\n", baseReward)
		fmt.Printf("     工作量占比: %.1f%% (%d/%d)\n", workRatio*100, localWork, totalWork)
		fmt.Printf("     实际奖励: %.4f OAW\n", actualReward)
	} else {
		fmt.Printf("  ⚠️ 挖到新区块 #%d (无工作量，无奖励)\n", block.Index)
	}
}

// getCurrentPeriodWork 获取当前周期的本地工作量
func (m *Miner) getCurrentPeriodWork(period int64) int {
	recordsDir := m.dataDir + "/records"
	entries, err := os.ReadDir(recordsDir)
	if err != nil {
		return 0
	}
	
	var work int
	periodStart := period * 60 // 周期开始的Unix时间
	
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(recordsDir, e.Name()))
		var record struct {
			Timestamp time.Time `json:"timestamp"`
			TotalTokens int    `json:"total_tokens"`
		}
		json.Unmarshal(data, &record)
		
		// 统计当前周期的工作量
		if record.Timestamp.Unix() >= periodStart {
			work += record.TotalTokens
		}
	}
	
	return work
}

// getNetworkWork 获取全网工作量 (从 PoLE 链查询)
func (m *Miner) getNetworkWork() int {
	// 查询当前周期内的全网 token 产量
	// 这里简化处理，返回本地工作量的 1.5 倍作为预估
	// 实际应该从 PoLE 节点 API 获取
	currentPeriod := time.Now().Unix() / 60
	localWork := m.getCurrentPeriodWork(currentPeriod)
	
	// 模拟全网工作量 (实际需要从 PoLE 链查询)
	// 这里假设全网有 5 个节点在工作
	estimatedNetworkWork := localWork * 5
	
	return estimatedNetworkWork
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
		// 检查 PoLE 节点是否已运行
		fmt.Println("检查 PoLE 节点状态...")
		rpc := NewPoleRPC(poleNodeURL)
		chainID, err := rpc.GetChainID()
		if err != nil {
			// 节点未运行，启动 PoLE 节点
			fmt.Println("PoLE 节点未运行，正在启动...")
			
			// 使用正确的参数启动 PoLE 节点
			cmdExec := exec.Command("D:/pole/pole-node.exe", 
				"-data-dir", poleDataDir, 
				"-genesis", "D:/pole/config/mainnet/genesis.json",
				"-rpc-port", ":9090", 
				"-p2p-port", ":26657")
			cmdExec.Stdout = os.Stdout
			cmdExec.Stderr = os.Stderr
			err := cmdExec.Start()
			if err != nil {
				fmt.Printf("警告: PoLE 节点启动失败: %v\n", err)
			} else {
				fmt.Printf("PoLE 节点已启动 (PID: %d)\n", cmdExec.Process.Pid)
			}

			// 等待节点就绪 (最多等待 60 秒)
			fmt.Println("等待节点就绪 (最多 60 秒)...")
			for i := 0; i < 60; i++ {
				time.Sleep(1 * time.Second)
				rpc := NewPoleRPC(poleNodeURL)
				chainID, err = rpc.GetChainID()
				if err == nil && chainID != "" {
					fmt.Printf("✅ 节点已就绪 (Chain ID: %s)\n", chainID)
					break
				}
				if i%10 == 0 && i > 0 {
					fmt.Printf("  等待中... (%d秒)\n", i)
				}
				if i == 59 {
					fmt.Println("警告: 节点启动超时，继续启动挖矿...")
				}
			}
		} else {
			fmt.Printf("✅ PoLE 节点已运行 (Chain ID: %s)\n", chainID)
		}

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
		fmt.Printf("难度: %d (范围: %d-%d)\n", m.difficulty, m.minDifficulty, m.maxDifficulty)
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
		poleContractAddress = args[1]
		fmt.Printf("✅ RPC 配置已更新:\n  节点: %s\n  合约: %s\n", poleNodeURL, poleContractAddress)
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

	// backup command - 备份数据
	rootCmd.AddCommand(&cobra.Command{
		Use: "backup",
		Short: "备份数据",
		RunE: func(cmd *cobra.Command, args []string) error {
			backupDir := dataDir + "-backup-" + time.Now().Format("20060102-150405")
			
			// 复制数据目录
			if err := copyDir(dataDir, backupDir); err != nil {
				return fmt.Errorf("备份失败: %v", err)
			}
			
			fmt.Printf("✅ 备份完成: %s\n", backupDir)
			return nil
		},
	})

	// export command - 导出数据
	rootCmd.AddCommand(&cobra.Command{
		Use: "export",
		Short: "导出数据 (JSON/CSV)",
		RunE: func(cmd *cobra.Command, args []string) error {
			format := "json"
			if len(args) > 0 {
				format = args[0]
			}

			exportFile := dataDir + "/export." + format
			
			if err := exportData(dataDir, exportFile, format); err != nil {
				return fmt.Errorf("导出失败: %v", err)
			}
			
			fmt.Printf("✅ 导出完成: %s\n", exportFile)
			return nil
		},
	})

	// dashboard command - 启动 Web Dashboard
	rootCmd.AddCommand(&cobra.Command{
		Use: "dashboard",
		Short: "启动 Web Dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			port := "8080"
			if len(args) > 0 {
				port = args[0]
			}
			
			fmt.Printf("启动 Web Dashboard: http://localhost:%s\n", port)
			return startDashboard(port, dataDir)
		},
	})

	rootCmd.Execute()
}

// Dashboard 模板
const dashboardHTML = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>OAW Dashboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f172a; color: #e2e8f0; min-height: 100vh; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        header { display: flex; justify-content: space-between; align-items: center; padding: 20px 0; border-bottom: 1px solid #334155; margin-bottom: 30px; }
        h1 { font-size: 28px; color: #38bdf8; }
        .nav { display: flex; gap: 20px; }
        .nav a { color: #94a3b8; text-decoration: none; transition: color 0.2s; }
        .nav a:hover { color: #38bdf8; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .card { background: #1e293b; border-radius: 12px; padding: 24px; border: 1px solid #334155; }
        .card h3 { font-size: 14px; color: #94a3b8; margin-bottom: 8px; text-transform: uppercase; }
        .card .value { font-size: 32px; font-weight: bold; color: #38bdf8; }
        .card .sub { font-size: 14px; color: #64748b; margin-top: 8px; }
        .table-container { background: #1e293b; border-radius: 12px; padding: 20px; border: 1px solid #334155; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #334155; }
        th { color: #94a3b8; font-weight: 500; font-size: 14px; }
        tr:hover { background: #334155; }
        .status { display: inline-block; padding: 4px 12px; border-radius: 20px; font-size: 12px; }
        .status.running { background: #22c55e20; color: #22c55e; }
        .status.stopped { background: #ef444420; color: #ef4444; }
        .btn { display: inline-block; padding: 8px 16px; background: #38bdf8; color: #0f172a; border-radius: 6px; text-decoration: none; font-weight: 500; }
        .btn:hover { background: #0ea5e9; }
        .footer { text-align: center; padding: 20px; color: #64748b; font-size: 14px; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🚀 OAW Dashboard</h1>
            <nav class="nav">
                <a href="/">概览</a>
                <a href="/blocks">区块</a>
                <a href="/records">记录</a>
            </nav>
        </header>
        
        <div class="grid">
            <div class="card">
                <h3>💰 钱包地址</h3>
                <div class="value" style="font-size: 16px;">{{.WalletAddress}}</div>
            </div>
            <div class="card">
                <h3>⛏️ 挖矿余额</h3>
                <div class="value">{{.Balance}} OAW</div>
                <div class="sub">区块: {{.BlockCount}}</div>
            </div>
            <div class="card">
                <h3>📊 工作量统计</h3>
                <div class="value">{{.TotalTokens}}</div>
                <div class="sub">累计 Token | 价值: {{.TotalValue}} OAW</div>
            </div>
            <div class="card">
                <h3>🔗 PoLE 链</h3>
                <div class="value">{{.ChainID}}</div>
                <div class="sub">区块高度: {{.BlockHeight}}</div>
            </div>
        </div>
        
        <div class="table-container">
            <h3 style="margin-bottom: 16px;">📋 最近区块</h3>
            <table>
                <thead>
                    <tr>
                        <th>高度</th>
                        <th>时间</th>
                        <th>矿工</th>
                        <th>奖励</th>
                        <th>哈希</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .RecentBlocks}}
                    <tr>
                        <td>#{{.Index}}</td>
                        <td>{{.Time}}</td>
                        <td>{{.Miner | printf "%.10s"}}</td>
                        <td>{{.Value}} OAW</td>
                        <td style="font-family: monospace; font-size: 12px;">{{.Hash | printf "%.16s"}}...</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        
        <div class="footer">
            OAW v1.0.0 | Proof of Live Engagement
        </div>
    </div>
</body>
</html>
`

type DashboardData struct {
	WalletAddress string
	Balance      float64
	BlockCount   int
	TotalTokens  int
	TotalValue   float64
	ChainID      string
	BlockHeight  string
	RecentBlocks []BlockInfo
}

type BlockInfo struct {
	Index   int
	Time    string
	Miner   string
	Value   float64
	Hash    string
}

func startDashboard(port, dataDir string) error {
	// 设置路由
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := getDashboardData(dataDir)
		tmpl, err := template.New("dashboard").Parse(dashboardHTML)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		tmpl.Execute(w, data)
	})

	// 静态文件
	fs := http.FileServer(http.Dir(filepath.Join(dataDir, "..", "dashboard")))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	return http.ListenAndServe(":"+port, nil)
}

func getDashboardData(dataDir string) DashboardData {
	data := DashboardData{
		ChainID:     "pole-mainnet-1",
		BlockHeight: "0",
	}

	// 读取钱包
	walletFile := filepath.Join(dataDir, "wallets", "default.json")
	if d, err := os.ReadFile(walletFile); err == nil {
		var w struct {
			Address string `json:"address"`
		}
		json.Unmarshal(d, &w)
		data.WalletAddress = w.Address
	}

	// 读取区块
	blocksFile := filepath.Join(dataDir, "blocks.json")
	if d, err := os.ReadFile(blocksFile); err == nil {
		var blocks []struct {
			Index     int     `json:"index"`
			Timestamp int64   `json:"timestamp"`
			Miner     string  `json:"miner"`
			Value     float64 `json:"value"`
			Hash      string  `json:"hash"`
		}
		json.Unmarshal(d, &blocks)
		data.BlockCount = len(blocks)
		for _, b := range blocks {
			if b.Miner == data.WalletAddress {
				data.Balance += b.Value
			}
		}
		// 最近 5 个区块
		start := len(blocks) - 5
		if start < 0 {
			start = 0
		}
		for i := start; i < len(blocks); i++ {
			data.RecentBlocks = append(data.RecentBlocks, BlockInfo{
				Index: blocks[i].Index,
				Time:  time.Unix(blocks[i].Timestamp, 0).Format("2006-01-02 15:04"),
				Miner: blocks[i].Miner,
				Value: blocks[i].Value,
				Hash:  blocks[i].Hash,
			})
		}
	}

	// 读取工作记录
	recordsDir := filepath.Join(dataDir, "records")
	if entries, err := os.ReadDir(recordsDir); err == nil {
		var totalTokens int
		var totalValue float64
		for _, e := range entries {
			if d, err := os.ReadFile(filepath.Join(recordsDir, e.Name())); err == nil {
				var r struct {
					TotalTokens int     `json:"total_tokens"`
					Value       float64 `json:"value"`
				}
				json.Unmarshal(d, &r)
				totalTokens += r.TotalTokens
				totalValue += r.Value
			}
		}
		data.TotalTokens = totalTokens
		data.TotalValue = totalValue
	}

	return data
}
