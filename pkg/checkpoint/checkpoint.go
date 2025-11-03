package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Position 表示 binlog 位置
type Position struct {
	File      string    `json:"file"`       // binlog 文件名
	Pos       uint32    `json:"pos"`        // 位置
	Timestamp time.Time `json:"timestamp"`  // 保存时间
	EventType string    `json:"event_type"` // 最后一个事件类型
	Database  string    `json:"database"`   // 最后一个数据库
	Table     string    `json:"table"`      // 最后一个表
}

// Manager checkpoint 管理器
type Manager struct {
	filePath string
	mu       sync.RWMutex
	current  *Position
}

// NewManager 创建 checkpoint 管理器
// source: 离线文件路径或在线 DSN
// sourceType: "file" 或 "mysql"
func NewManager(source, sourceType string) *Manager {
	homeDir, _ := os.UserHomeDir()
	checkpointDir := filepath.Join(homeDir, ".binlogx", "checkpoints")
	os.MkdirAll(checkpointDir, 0755)

	// 根据数据源类型和内容生成唯一的文件名
	var fileName string
	if sourceType == "file" {
		// 离线文件：使用文件的绝对路径的 hash
		absPath, _ := filepath.Abs(source)
		fileName = fmt.Sprintf("file_%x.json", hashString(absPath))
	} else {
		// 在线服务：使用 DSN 的 hash
		fileName = fmt.Sprintf("mysql_%x.json", hashString(source))
	}

	filePath := filepath.Join(checkpointDir, fileName)

	return &Manager{
		filePath: filePath,
	}
}

// Load 加载上次保存的位置
func (m *Manager) Load() (*Position, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 文件不存在，返回 nil 表示没有保存的位置
		}
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var pos Position
	if err := json.Unmarshal(data, &pos); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &pos, nil
}

// Save 保存当前位置
func (m *Manager) Save(file string, pos uint32, eventType, database, table string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	position := &Position{
		File:      file,
		Pos:       pos,
		Timestamp: time.Now(),
		EventType: eventType,
		Database:  database,
		Table:     table,
	}

	data, err := json.MarshalIndent(position, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(m.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	m.current = position
	return nil
}

// Clear 清除保存的位置
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.Remove(m.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove checkpoint file: %w", err)
	}

	m.current = nil
	return nil
}

// GetCurrent 获取当前位置
func (m *Manager) GetCurrent() *Position {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// hashString 简单的字符串 hash 函数
func hashString(s string) uint32 {
	h := uint32(0)
	for i := 0; i < len(s); i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}
