package openclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OpenClaw OpenClaw 集成
type OpenClaw struct {
	URL   string
	Token string
}

// New 创建 OpenClaw 客户端
func New(url, token string) *OpenClaw {
	return &OpenClaw{URL: url, Token: token}
}

// Session 会话
type Session struct {
	SessionID    string `json:"sessionId"`
	UpdatedAt    int64  `json:"updatedAt"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	TotalTokens  int    `json:"totalTokens"`
	Model        string `json:"model"`
	AgentID      string `json:"agentId"`
	Kind         string `json:"kind"`
}

// GetSessions 获取会话列表
func GetSessions() (map[string]Session, error) {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home = os.Getenv("HOME")
	}
	sessionPath := filepath.Join(home, ".openclaw", "agents", "main", "sessions", "sessions.json")

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, err
	}

	// sessions.json 是 map[string]Session 格式
	var sessions map[string]Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}

	return sessions, nil
}

// WorkRecord 工作量记录
type WorkRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	SessionID    string    `json:"session_id"`
	AgentID      string    `json:"agent_id"`
	Kind         string    `json:"kind"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	Value        float64   `json:"value"`
}

// CalculateValue 计算工作量价值
func CalculateValue(record WorkRecord) float64 {
	// 价值公式：
	// - 输出 token = AI 创造的价值
	// - 输入 token = 消耗的成本
	// 输出 token 价值远高于输入成本（鼓励 AI 干活）
	outputValue := float64(record.OutputTokens) * 0.1   // 每个输出 token 值 0.1
	inputCost := float64(record.InputTokens) * 0.001    // 每个输入 token 成本 0.001
	
	// 任务类型加成
	bonus := 1.0
	if record.Kind == "cron" {
		bonus = 1.5 // 定时任务加成（自动执行）
	}
	
	return (outputValue - inputCost) * bonus
}

// SaveRecord 保存记录
func SaveRecord(dir string, record WorkRecord) error {
	os.MkdirAll(dir, 0755)
	filename := filepath.Join(dir, fmt.Sprintf("%d.json", time.Now().UnixNano()))
	data, _ := json.MarshalIndent(record, "", "  ")
	return os.WriteFile(filename, data, 0644)
}

// LoadRecords 加载记录
func LoadRecords(dir string) ([]WorkRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var records []WorkRecord
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		var record WorkRecord
		if json.Unmarshal(data, &record) == nil {
			records = append(records, record)
		}
	}
	return records, nil
}

// SyncFromSessions 从 OpenClaw 同步工作量
func SyncFromSessions(dataDir string) error {
	sessions, err := GetSessions()
	if err != nil {
		return fmt.Errorf("读取会话失败: %w", err)
	}

	fmt.Printf("获取到 %d 条会话记录\n", len(sessions))

	totalValue := 0.0
	for key, s := range sessions {
		// 从 key 提取 kind (direct/cron)
		kind := "direct"
		if len(key) > 5 && key[:5] == "cron:" {
			kind = "cron"
		}
		
		record := WorkRecord{
			Timestamp:    time.UnixMilli(s.UpdatedAt),
			SessionID:    s.SessionID,
			AgentID:      s.AgentID,
			Kind:         kind,
			InputTokens:  s.InputTokens,
			OutputTokens: s.OutputTokens,
			TotalTokens:  s.TotalTokens,
		}
		record.Value = CalculateValue(record)
		
		SaveRecord(dataDir+"/records", record)
		totalValue += record.Value
	}

	fmt.Printf("总价值: %.2f OAW\n", totalValue)
	return nil
}

// GetTotalStats 获取总统计数据
func GetTotalStats(dataDir string) (totalTokens int, totalValue float64, err error) {
	records, err := LoadRecords(dataDir + "/records")
	if err != nil {
		return 0, 0, err
	}

	for _, r := range records {
		totalTokens += r.TotalTokens
		totalValue += r.Value
	}
	return totalTokens, totalValue, nil
}
