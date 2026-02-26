package openclaw

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// SessionSummary 会话摘要
type SessionSummary struct {
	AgentID     string    `json:"agent_id"`
	SessionID   string    `json:"session_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	TokenUsed   int       `json:"token_used"`
	TasksDone   int       `json:"tasks_done"`
	FilesCreate int       `json:"files_created"`
	LinesCode   int       `json:"lines_code"`
}

// GetWorkSummary 获取工作量摘要
func (o *OpenClaw) GetWorkSummary() ([]SessionSummary, error) {
	url := o.URL + "/api/sessions"
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API 返回: %d", resp.StatusCode)
	}

	var summaries []SessionSummary
	if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
		return nil, err
	}

	return summaries, nil
}

// WorkRecord 工作量记录
type WorkRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	AgentID    string    `json:"agent_id"`
	TaskType   string    `json:"task_type"`
	Tokens     int       `json:"tokens"`
	Files      int       `json:"files"`
	LinesCode  int       `json:"lines_code"`
	TasksDone  int       `json:"tasks_done"`
	Value      float64   `json:"value"`
}

// CalculateValue 计算工作量价值
func CalculateValue(record WorkRecord) float64 {
	// 价值公式
	base := float64(record.TasksDone) * 10
	tokens := float64(record.Tokens) * 0.0001
	code := float64(record.LinesCode) * 0.01
	files := float64(record.Files) * 5

	return base + code + files - tokens
}

// SaveRecord 保存记录到文件
func SaveRecord(dir string, record WorkRecord) error {
	os.MkdirAll(dir, 0755)
	
	filename := filepath.Join(dir, fmt.Sprintf("%d.json", time.Now().Unix()))
	data, _ := json.MarshalIndent(record, "", "  ")
	return os.WriteFile(filename, data, 0644)
}

// LoadRecords 从目录加载所有记录
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
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var record WorkRecord
		if json.Unmarshal(data, &record) == nil {
			records = append(records, record)
		}
	}
	return records, nil
}

// FetchAndRecord 从 OpenClaw 获取工作量并记录
func (o *OpenClaw) FetchAndRecord(dataDir string) error {
	summaries, err := o.GetWorkSummary()
	if err != nil {
		return err
	}

	for _, s := range summaries {
		record := WorkRecord{
			Timestamp:  s.EndTime,
			AgentID:    s.AgentID,
			TasksDone:  s.TasksDone,
			Tokens:     s.TokenUsed,
			LinesCode:  s.LinesCode,
			Files:      s.FilesCreate,
		}
		record.Value = CalculateValue(record)
		
		SaveRecord(dataDir+"/records", record)
	}

	return nil
}
