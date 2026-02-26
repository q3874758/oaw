package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

var (
	logger       = zerolog.New(os.Stderr).With().Timestamp().Logger()
	dataDir     string
	port        int
	openclawURL string
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "启动 OAW 服务",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().StringVar(&dataDir, "datadir", "./data", "数据目录")
	startCmd.Flags().IntVar(&port, "port", 18790, "API 端口")
	startCmd.Flags().StringVar(&openclawURL, "openclaw", "http://localhost:18789", "OpenClaw URL")
}

func runStart(cmd *cobra.Command, args []string) error {
	logger.Info().
		Str("version", version).
		Str("port", fmt.Sprintf("%d", port)).
		Msg("启动 OAW 服务")

	// 创建数据目录
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	// 启动 API 服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动 OpenClaw 集成器
	go startIntegrator(ctx)

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigCh:
		logger.Info().Msg("正在关闭...")
	case <-ctx.Done():
	}

	return nil
}

func startIntegrator(ctx context.Context) {
	logger.Info().Str("url", openclawURL).Msg("启动 OpenClaw 集成器")
	
	// 模拟：每隔 30 秒记录一次工作量
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// TODO: 实际集成 OpenClaw 获取工作量
			logger.Debug().Msg("同步工作量数据...")
		}
	}
}
