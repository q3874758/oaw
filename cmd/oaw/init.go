package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化 OAW 工作目录",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	dir := dataDir
	if dir == "" {
		dir = "./data"
	}

	dirs := []string{
		dir,
		dir + "/records",
		dir + "/proofs",
		dir + "/cache",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", d, err)
		}
		fmt.Printf("✓ 创建 %s\n", d)
	}

	// 创建配置文件
	config := `{
  "version": "1.0.0",
  "port": 18790,
  "openclaw_url": "http://localhost:18789",
  "metrics": {
    "enabled": true,
    "interval": 30
  }
}`
	
	configFile := dir + "/config.json"
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	fmt.Printf("✓ 创建 %s\n", configFile)

	fmt.Println("\n初始化完成！运行 'oaw start' 启动服务")
	return nil
}
