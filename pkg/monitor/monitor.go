package monitor

import (
	"fmt"
	"log"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
)

// Monitor 用于监控慢方法和大事件
type Monitor struct {
	slowThreshold      time.Duration
	eventSizeThreshold int64

	// 统计数据
	slowMethodCount int64
	largeEventCount int64
	maxEventSize    int64
}

// NewMonitor 创建监控器
func NewMonitor(slowThreshold time.Duration, eventSizeThreshold int64) *Monitor {
	return &Monitor{
		slowThreshold:      slowThreshold,
		eventSizeThreshold: eventSizeThreshold,
	}
}

// LogSlowMethod 记录慢方法（计算从 startTime 开始的耗时）
// 格式: [SLOW] 方法名 耗时 入参
func (m *Monitor) LogSlowMethod(methodName string, startTime time.Time, args string) {
	duration := time.Since(startTime)
	if duration > m.slowThreshold {
		m.slowMethodCount++
		log.Printf("[SLOW] %s took %v, args: %s\n", methodName, duration, args)
	}
}

// CheckEventSize 检查事件大小
// 格式: [WARN] Large event detected: log-pos=... type=... size=... bytes
func (m *Monitor) CheckEventSize(event *models.Event) {
	if m.eventSizeThreshold <= 0 {
		return
	}

	eventSize := int64(len(event.RawData))
	if eventSize > m.eventSizeThreshold {
		m.largeEventCount++
		if eventSize > m.maxEventSize {
			m.maxEventSize = eventSize
		}
		log.Printf("[WARN] Large event detected: log-pos=%d, type=%s, size=%d bytes\n",
			event.LogPos, event.EventType, eventSize)
	}
}

// CheckEventsSizeBatch 批量检查事件大小
func (m *Monitor) CheckEventsSizeBatch(events []*models.Event) {
	for _, event := range events {
		m.CheckEventSize(event)
	}
}

// GetStats 获取监控统计数据
func (m *Monitor) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"slow_threshold":       m.slowThreshold,
		"event_size_threshold": m.eventSizeThreshold,
		"slow_method_count":    m.slowMethodCount,
		"large_event_count":    m.largeEventCount,
		"max_event_size":       m.maxEventSize,
	}
}

// PrintStats 打印监控统计信息
func (m *Monitor) PrintStats() {
	if m.slowMethodCount > 0 || m.largeEventCount > 0 {
		fmt.Println("\n=== Monitor Statistics ===")
		if m.slowMethodCount > 0 {
			fmt.Printf("Slow methods detected: %d\n", m.slowMethodCount)
			fmt.Printf("Threshold: %v\n", m.slowThreshold)
		}
		if m.largeEventCount > 0 {
			fmt.Printf("Large events detected: %d\n", m.largeEventCount)
			fmt.Printf("Max event size: %d bytes\n", m.maxEventSize)
			fmt.Printf("Threshold: %d bytes\n", m.eventSizeThreshold)
		}
	}
}
