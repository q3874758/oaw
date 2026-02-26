package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看 OAW 状态",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	dir := dataDir
	if dir == "" {
		dir = "./data"
	}

	configFile := dir + "/config.json"
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	fmt.Println("=== OAW 状态 ===")
	fmt.Printf("版本: %s\n", version)
	fmt.Printf("数据目录: %s\n", dir)
	
	if v, ok := config["port"]; ok {
		fmt.Printf("API 端口: %v\n", v)
	}
	if v, ok := config["openclaw_url"]; ok {
		fmt.Printf("OpenClaw URL: %v\n", v)
	}

	// 检查记录文件
	recordsDir := dir + "/records"
	if entries, err := os.ReadDir(recordsDir); err == nil {
		fmt.Printf("\n工作量记录: %d 条\n", len(entries))
	}

	proofsDir := dir + "/proofs"
	if entries, err := os.ReadDir(proofsDir); err == nil {
		fmt.Printf("工作证明: %d 条\n", len(entries))
	}

	return nil
}
