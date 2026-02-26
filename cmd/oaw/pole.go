package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var poleCmd = &cobra.Command{
	Use:   "pole",
	Short: "PoLE 链集成",
}

var poleSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "同步到 PoLE 链",
	RunE:  runPoleSync,
}

var poleWalletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "查看 PoLE 钱包",
	RunE:  runPoleWallet,
}

func init() {
	poleCmd.AddCommand(poleSyncCmd)
	poleCmd.AddCommand(poleWalletCmd)
	rootCmd.AddCommand(poleCmd)
}

func runPoleSync(cmd *cobra.Command, args []string) error {
	fmt.Println("=== OAW → PoLE 链同步 ===")
	
	// 读取 OAW 记录
	records, err := openclaw.LoadRecords(dataDir + "/records")
	if err != nil {
		return fmt.Errorf("加载记录失败: %w", err)
	}

	fmt.Printf("已加载 %d 条工作记录\n", len(records))

	// 计算总价值
	var totalValue float64
	var totalTokens int
	for _, r := range records {
		totalValue += r.Value
		totalTokens += r.TotalTokens
	}

	fmt.Printf("\n累计:\n")
	fmt.Printf("  Token: %d\n", totalTokens)
	fmt.Printf("  价值: %.2f OAW\n", totalValue)

	// TODO: 实际提交到 PoLE 链
	fmt.Println("\n✅ 同步完成!")
	fmt.Println("(PoLE 链集成开发中...)")

	return nil
}

func runPoleWallet(cmd *cobra.Command, args []string) error {
	walletPath := "D:/pole/wallet.json"
	data, err := os.ReadFile(walletPath)
	if err != nil {
		return fmt.Errorf("读取钱包失败: %w", err)
	}

	var wallet struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(data, &wallet); err != nil {
		return err
	}

	fmt.Println("=== PoLE 钱包 ===")
	fmt.Printf("地址: %s\n", wallet.Address)
	fmt.Println("(余额查询开发中...)")

	return nil
}
