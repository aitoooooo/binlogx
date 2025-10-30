package monitor

import (
	"testing"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
)

func TestLogSlowMethod(t *testing.T) {
	monitor := NewMonitor(100*time.Millisecond, 0)

	// 慢方法应该被计数
	slowStart := time.Now().Add(-200 * time.Millisecond)
	monitor.LogSlowMethod("slowFunc", slowStart, "arg1=test")
	if monitor.slowMethodCount != 1 {
		t.Errorf("Expected slowMethodCount to be 1, got %d", monitor.slowMethodCount)
	}

	// 快速方法不应该被计数
	fastStart := time.Now().Add(-50 * time.Millisecond)
	monitor.LogSlowMethod("fastFunc", fastStart, "arg1=test")
	if monitor.slowMethodCount != 1 {
		t.Errorf("Expected slowMethodCount to still be 1, got %d", monitor.slowMethodCount)
	}
}

func TestCheckEventSize(t *testing.T) {
	eventSizeThreshold := int64(100)
	monitor := NewMonitor(1*time.Second, eventSizeThreshold)

	// 小事件不应该被计数
	smallEvent := &models.Event{
		LogPos:    100,
		EventType: "WRITE_ROWS",
		RawData:   make([]byte, 50),
	}
	monitor.CheckEventSize(smallEvent)
	if monitor.largeEventCount != 0 {
		t.Errorf("Expected largeEventCount to be 0, got %d", monitor.largeEventCount)
	}

	// 大事件应该被计数
	largeEvent := &models.Event{
		LogPos:    200,
		EventType: "WRITE_ROWS",
		RawData:   make([]byte, 150),
	}
	monitor.CheckEventSize(largeEvent)
	if monitor.largeEventCount != 1 {
		t.Errorf("Expected largeEventCount to be 1, got %d", monitor.largeEventCount)
	}

	if monitor.maxEventSize != 150 {
		t.Errorf("Expected maxEventSize to be 150, got %d", monitor.maxEventSize)
	}

	// 更大的事件应该更新最大值
	veryLargeEvent := &models.Event{
		LogPos:    300,
		EventType: "WRITE_ROWS",
		RawData:   make([]byte, 200),
	}
	monitor.CheckEventSize(veryLargeEvent)
	if monitor.largeEventCount != 2 {
		t.Errorf("Expected largeEventCount to be 2, got %d", monitor.largeEventCount)
	}
	if monitor.maxEventSize != 200 {
		t.Errorf("Expected maxEventSize to be 200, got %d", monitor.maxEventSize)
	}
}

func TestCheckEventSizeBatch(t *testing.T) {
	eventSizeThreshold := int64(100)
	monitor := NewMonitor(1*time.Second, eventSizeThreshold)

	events := []*models.Event{
		{
			LogPos:    100,
			EventType: "WRITE_ROWS",
			RawData:   make([]byte, 50),
		},
		{
			LogPos:    200,
			EventType: "WRITE_ROWS",
			RawData:   make([]byte, 150),
		},
		{
			LogPos:    300,
			EventType: "UPDATE_ROWS",
			RawData:   make([]byte, 200),
		},
	}

	monitor.CheckEventsSizeBatch(events)
	if monitor.largeEventCount != 2 {
		t.Errorf("Expected largeEventCount to be 2, got %d", monitor.largeEventCount)
	}
	if monitor.maxEventSize != 200 {
		t.Errorf("Expected maxEventSize to be 200, got %d", monitor.maxEventSize)
	}
}

func TestGetStats(t *testing.T) {
	monitor := NewMonitor(100*time.Millisecond, 1024)
	monitor.slowMethodCount = 5
	monitor.largeEventCount = 3
	monitor.maxEventSize = 2048

	stats := monitor.GetStats()

	if v, ok := stats["slow_method_count"]; !ok || v != int64(5) {
		t.Errorf("Expected slow_method_count to be 5, got %v", v)
	}
	if v, ok := stats["large_event_count"]; !ok || v != int64(3) {
		t.Errorf("Expected large_event_count to be 3, got %v", v)
	}
	if v, ok := stats["max_event_size"]; !ok || v != int64(2048) {
		t.Errorf("Expected max_event_size to be 2048, got %v", v)
	}
}

func TestEventSizeThresholdDisabled(t *testing.T) {
	monitor := NewMonitor(1*time.Second, 0) // threshold = 0 means disabled

	largeEvent := &models.Event{
		LogPos:    100,
		EventType: "WRITE_ROWS",
		RawData:   make([]byte, 10000),
	}
	monitor.CheckEventSize(largeEvent)
	if monitor.largeEventCount != 0 {
		t.Errorf("Expected largeEventCount to be 0 when threshold is 0, got %d", monitor.largeEventCount)
	}
}
