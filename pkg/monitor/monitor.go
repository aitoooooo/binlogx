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

// LogSlowMethod 记录慢方法
// 格式: [SLOW] 方法名 耗时 入参
func (m *Monitor) LogSlowMethod(methodName string, duration time.Duration, args string) {
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

// Timeit 用于测量执行时间（适用于处理函数）
// 记录执行时间并在超过阈值时输出日志
func (m *Monitor) Timeit(methodName string, args string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)
	m.LogSlowMethod(methodName, duration, args)
	return err
}

// TimeitWithResult 用于测量执行时间并返回结果
// 返回结果和错误，同时记录执行时间
func (m *Monitor) TimeitWithResult(methodName string, args string, fn func() (interface{}, error)) (interface{}, error) {
	start := time.Now()
	result, err := fn()
	duration := time.Since(start)
	m.LogSlowMethod(methodName, duration, args)
	return result, err
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
