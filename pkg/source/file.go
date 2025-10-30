package source

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/go-mysql-org/go-mysql/replication"
)

// FileSource 离线 binlog 文件数据源
type FileSource struct {
	filePath  string
	parser    *replication.BinlogParser
	streamer  *replication.BinlogStreamer
	eof       bool
	eventChan chan *replication.BinlogEvent
	errChan   chan error
	mu        sync.RWMutex
	startTime time.Time
	endTime   time.Time
}

// NewFileSource 创建文件数据源
func NewFileSource(filePath string) *FileSource {
	return &FileSource{
		filePath: filePath,
	}
}

// SetTimeRange 设置时间范围过滤
func (fs *FileSource) SetTimeRange(start, end time.Time) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.startTime = start
	fs.endTime = end
}

// Open 打开文件并初始化解析器
func (fs *FileSource) Open(ctx context.Context) error {
	// 创建 binlog 解析器
	parser := replication.NewBinlogParser()

	// 创建流处理器
	fs.streamer = replication.NewBinlogStreamer()
	fs.eof = false
	fs.eventChan = make(chan *replication.BinlogEvent, 100)
	fs.errChan = make(chan error, 1)

	// 启动异步事件读取
	go fs.readEvents(ctx, parser)

	return nil
}

// readEvents 后台读取事件的 goroutine
func (fs *FileSource) readEvents(ctx context.Context, parser *replication.BinlogParser) {
	defer close(fs.eventChan)
	defer close(fs.errChan)

	// 定义事件回调函数，在解析器中被调用
	onEvent := func(e *replication.BinlogEvent) error {
		select {
		case fs.eventChan <- e:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}

	// 解析文件，使用回调处理每个事件
	if err := parser.ParseFile(fs.filePath, 0, onEvent); err != nil {
		fs.mu.Lock()
		fs.eof = true
		fs.mu.Unlock()
		fs.errChan <- fmt.Errorf("failed to parse binlog file: %w", err)
		return
	}

	fs.mu.Lock()
	fs.eof = true
	fs.mu.Unlock()
}

// Close 关闭文件
func (fs *FileSource) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.eof = true
	return nil
}

// Read 读取下一个事件并转换为内部模型
func (fs *FileSource) Read() (*models.Event, error) {
	fs.mu.RLock()
	eof := fs.eof
	fs.mu.RUnlock()

	if eof && len(fs.eventChan) == 0 {
		return nil, fmt.Errorf("EOF")
	}

	select {
	case event, ok := <-fs.eventChan:
		if !ok {
			return nil, fmt.Errorf("EOF")
		}
		return fs.convertEvent(event)
	case err := <-fs.errChan:
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("EOF")
	}
}

// HasMore 是否还有更多数据
func (fs *FileSource) HasMore() bool {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return !fs.eof || len(fs.eventChan) > 0
}

// convertEvent 将 go-mysql 的事件转换为内部模型
func (fs *FileSource) convertEvent(event *replication.BinlogEvent) (*models.Event, error) {
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}

	internalEvent := &models.Event{
		Timestamp: time.Unix(int64(event.Header.Timestamp), 0),
		EventType: event.Header.EventType.String(),
		ServerID:  event.Header.ServerID,
		LogPos:    event.Header.LogPos,
		RawData:   event.RawData,
	}

	// 检查时间范围
	fs.mu.RLock()
	startTime := fs.startTime
	endTime := fs.endTime
	fs.mu.RUnlock()

	if !startTime.IsZero() && internalEvent.Timestamp.Before(startTime) {
		return nil, fmt.Errorf("before start time")
	}
	if !endTime.IsZero() && internalEvent.Timestamp.After(endTime) {
		return nil, fmt.Errorf("after end time")
	}

	// 根据事件类型解析详细内容
	switch e := event.Event.(type) {
	case *replication.RowsEvent:
		return fs.parseRowsEvent(internalEvent, e, event.Header)
	case *replication.QueryEvent:
		internalEvent.SQL = string(e.Query)
		internalEvent.Database = string(e.Schema)
		internalEvent.Action = "QUERY"
	}

	return internalEvent, nil
}

// parseRowsEvent 解析行事件
func (fs *FileSource) parseRowsEvent(event *models.Event, e *replication.RowsEvent, header *replication.EventHeader) (*models.Event, error) {
	// 从 TableMapEvent 获取数据库和表信息
	if e.Table == nil {
		return event, fmt.Errorf("missing table map event")
	}

	event.Database = string(e.Table.Schema)
	event.Table = string(e.Table.Table)

	// 根据事件类型确定操作类型
	switch header.EventType {
	case replication.WRITE_ROWS_EVENTv1, replication.WRITE_ROWS_EVENTv2:
		event.Action = "INSERT"
	case replication.UPDATE_ROWS_EVENTv1, replication.UPDATE_ROWS_EVENTv2:
		event.Action = "UPDATE"
	case replication.DELETE_ROWS_EVENTv1, replication.DELETE_ROWS_EVENTv2:
		event.Action = "DELETE"
	}

	// 解析列和值
	if len(e.Rows) > 0 {
		event.AfterValues = fs.rowToMap(e.Rows[0], e.Table)
		if len(e.Rows) > 1 && event.Action == "UPDATE" {
			event.BeforeValues = fs.rowToMap(e.Rows[1], e.Table)
		} else if len(e.Rows) > 1 && (event.Action == "DELETE" || event.Action == "UPDATE") {
			// For DELETE and UPDATE, first row might be the before image
			event.BeforeValues = fs.rowToMap(e.Rows[0], e.Table)
			if len(e.Rows) > 1 {
				event.AfterValues = fs.rowToMap(e.Rows[1], e.Table)
			}
		}
	}

	return event, nil
}

// rowToMap 将行数据转换为 map
func (fs *FileSource) rowToMap(row []interface{}, tableMap *replication.TableMapEvent) map[string]interface{} {
	if row == nil || tableMap == nil {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})

	for i, col := range row {
		if i < int(tableMap.ColumnCount) {
			// 获取列名（暂时使用 col_N，因为 go-mysql 不提供列名）
			colName := fmt.Sprintf("col_%d", i)
			result[colName] = col
		}
	}

	return result
}
