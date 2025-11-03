package processor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/source"
)

const defaultBufferSize = 10000

// EventProcessor 生产者-消费者事件处理器
type EventProcessor struct {
	dataSource     source.DataSource
	filter         *filter.RouteFilter
	workerCount    int
	bufferSize     int
	workerChannels []chan *models.Event
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	handlers       []EventHandler
	mu             sync.RWMutex
}

// EventHandler 事件处理器接口
type EventHandler interface {
	Handle(event *models.Event) error
	Flush() error
}

// NewEventProcessor 创建事件处理器
func NewEventProcessor(
	dataSource source.DataSource,
	filter *filter.RouteFilter,
	workerCount int,
) *EventProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	// 为每个 worker 创建一个 channel
	workerChannels := make([]chan *models.Event, workerCount)
	for i := 0; i < workerCount; i++ {
		workerChannels[i] = make(chan *models.Event, defaultBufferSize)
	}

	return &EventProcessor{
		dataSource:     dataSource,
		filter:         filter,
		workerCount:    workerCount,
		bufferSize:     defaultBufferSize,
		workerChannels: workerChannels,
		ctx:            ctx,
		cancel:         cancel,
		handlers:       make([]EventHandler, 0),
	}
}

// AddHandler 添加事件处理器
func (ep *EventProcessor) AddHandler(handler EventHandler) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	ep.handlers = append(ep.handlers, handler)
}

// Start 启动处理
func (ep *EventProcessor) Start() error {
	// 启动生产者
	ep.wg.Add(1)
	go ep.producer()

	// 启动消费者
	for i := 0; i < ep.workerCount; i++ {
		ep.wg.Add(1)
		go ep.consumer(i)
	}

	return nil
}

// Wait 等待处理完成
func (ep *EventProcessor) Wait() error {
	ep.wg.Wait()
	return ep.flush()
}

// Stop 停止处理
func (ep *EventProcessor) Stop() {
	ep.cancel()
}

// producer 生产者：读取事件并写入对应的 worker channel
func (ep *EventProcessor) producer() {
	defer ep.wg.Done()
	defer func() {
		// 关闭所有 worker channels
		for i := 0; i < ep.workerCount; i++ {
			close(ep.workerChannels[i])
		}
	}()

	for ep.dataSource.HasMore() {
		event, err := ep.dataSource.Read()
		if err != nil {
			// 检查是否是 EOF 错误，EOF 是正常的结束信号，不需要打印错误日志
			if err.Error() == "EOF" {
				// 文件已读完，正常退出循环
				log.Printf("reading event over: %v\n", err)
				break
			}
			// 其他错误才记录日志
			log.Printf("Error reading event: %v\n", err)
			break
		}

		if event == nil {
			// 暂时没有事件，继续尝试
			time.Sleep(time.Second)
			continue
		}

		// 过滤
		if !ep.filter.Match(event) {
			continue
		}

		// 根据 table 和 key 计算应该路由到哪个 worker
		workerID := ep.filter.GetWorkerID(event.Table, getEventKey(event), ep.workerCount)

		// 发送到对应的 worker channel
		select {
		case ep.workerChannels[workerID] <- event:
		case <-ep.ctx.Done():
			return
		}
	}
}

// consumer 消费者：处理事件（只从自己的 channel 读取）
func (ep *EventProcessor) consumer(id int) {
	defer ep.wg.Done()

	for {
		select {
		case event, ok := <-ep.workerChannels[id]:
			if !ok {
				return
			}

			if event == nil {
				continue
			}

			// 处理事件
			ep.handleEvent(event)

		case <-ep.ctx.Done():
			return
		}
	}
}

// handleEvent 处理单个事件
func (ep *EventProcessor) handleEvent(event *models.Event) {
	// 注意：handlers 在 Start() 后就不会改变，所以可以在消费者中直接访问
	// 避免每个事件都要获取 RLock，大幅降低锁竞争
	for _, handler := range ep.handlers {
		if err := handler.Handle(event); err != nil {
			log.Printf("Error handling event: %v\n", err)
		}
	}
}

// flush 刷新所有处理器
func (ep *EventProcessor) flush() error {
	// 注意：handlers 在 Start() 后就不会改变，所以可以直接访问
	for _, handler := range ep.handlers {
		if err := handler.Flush(); err != nil {
			return fmt.Errorf("error flushing handler: %w", err)
		}
	}
	return nil
}

// getEventKey 获取事件的唯一键（用于路由）
func getEventKey(event *models.Event) string {
	// 使用 table + 前 5 个 after values 作为键
	key := event.Table
	for _, v := range event.AfterValues {
		key += fmt.Sprintf(":%v", v)
		break
	}
	return key
}
