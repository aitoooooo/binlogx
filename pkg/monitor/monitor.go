package monitor

import (
	"log"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
)

// Monitor 用于监控慢方法和大事件
type Monitor struct {
	slowThreshold      time.Duration
	eventSizeThreshold int64
}

// NewMonitor 创建监控器
func NewMonitor(slowThreshold time.Duration, eventSizeThreshold int64) *Monitor {
	return &Monitor{
		slowThreshold:      slowThreshold,
		eventSizeThreshold: eventSizeThreshold,
	}
}

// LogSlowMethod 记录慢方法
func (m *Monitor) LogSlowMethod(methodName string, duration time.Duration, args string) {
	if duration > m.slowThreshold {
		log.Printf("[SLOW] %s took %v, args: %s\n", methodName, duration, args)
	}
}

// CheckEventSize 检查事件大小
func (m *Monitor) CheckEventSize(event *models.Event) {
	if m.eventSizeThreshold <= 0 {
		return
	}

	eventSize := int64(len(event.RawData))
	if eventSize > m.eventSizeThreshold {
		log.Printf("[WARN] Large event detected: log-pos=%d, type=%s, size=%d bytes\n",
			event.LogPos, event.EventType, eventSize)
	}
}

// Timeit 用于测量执行时间
func (m *Monitor) Timeit(methodName string, args string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)
	m.LogSlowMethod(methodName, duration, args)
	return err
}
