package source

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/go-mysql-org/go-mysql/replication"
)

// FileSource 离线 binlog 文件数据源
type FileSource struct {
	filePath   string
	parser     *replication.BinlogParser
	streamer   *replication.BinlogStreamer
	eof        bool
	eventChan  chan *replication.BinlogEvent
	errChan    chan error
	mu         sync.RWMutex
	startTime  time.Time
	endTime    time.Time
	startPos   uint32 // 断点续看的起始位置
	startFile  string // 断点续看的起始文件（用于多文件场景）
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

// SetStartPosition 设置起始位置（用于断点续看）
func (fs *FileSource) SetStartPosition(file string, pos uint32) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.startFile = file
	fs.startPos = pos
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

	// 获取起始位置配置
	fs.mu.RLock()
	startPos := fs.startPos
	fs.mu.RUnlock()

	// 定义事件回调函数，在解析器中被调用
	onEvent := func(e *replication.BinlogEvent) error {
		// 如果指定了起始位置，跳过在这个位置之前的事件
		// 注意：LogPos 是事件的结束位置，所以我们需要 > startPos 而不是 >= startPos
		if startPos > 0 && e.Header.LogPos <= startPos {
			return nil // 跳过此事件，因为它在断点位置之前或就是断点位置
		}

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
	hasEvents := len(fs.eventChan) > 0
	fs.mu.RUnlock()

	// 如果已到文件末尾且没有缓冲事件，直接返回错误
	if eof && !hasEvents {
		return nil, fmt.Errorf("EOF")
	}

	// 非阻塞读取：先尝试从 eventChan 读取
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
	default:
		// 没有立即可用的事件
		if eof {
			return nil, fmt.Errorf("EOF")
		}
		// 让出 CPU，短暂等待事件到达
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
		case <-time.After(100 * time.Millisecond):
			// 超时仍未收到事件，返回 nil 而不是继续阻塞
			return nil, nil
		}
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
		LogName:   filepath.Base(fs.filePath), // 从文件路径提取文件名
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
		event.AfterValues = fs.rowToMap(e.Rows[0], e)
		if len(e.Rows) > 1 && event.Action == "UPDATE" {
			event.BeforeValues = fs.rowToMap(e.Rows[1], e)
		} else if len(e.Rows) > 1 && (event.Action == "DELETE" || event.Action == "UPDATE") {
			// For DELETE and UPDATE, first row might be the before image
			event.BeforeValues = fs.rowToMap(e.Rows[0], e)
			if len(e.Rows) > 1 {
				event.AfterValues = fs.rowToMap(e.Rows[1], e)
			}
		}
	}

	return event, nil
}

// rowToMap 将行数据转换为 map
// 注意：RowsEvent.Rows 中的数据与 ColumnBitmap1 对应
// ColumnBitmap1 指示了哪些列被包含在行数据中
func (fs *FileSource) rowToMap(row []interface{}, rowsEvent *replication.RowsEvent) map[string]interface{} {
	if row == nil || rowsEvent == nil || rowsEvent.Table == nil {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})

	// 根据 ColumnBitmap1 确定哪些列被包含
	// ColumnBitmap1 是一个字节数组，每一位表示对应列是否被包含
	includedCols := getIncludedColumnIndices(int(rowsEvent.Table.ColumnCount), rowsEvent.ColumnBitmap1)

	// 将 row 中的数据按照 includedCols 映射到正确的列号
	for i, col := range row {
		if i < len(includedCols) {
			colName := fmt.Sprintf("col_%d", includedCols[i])
			result[colName] = col
		}
	}

	return result
}

// getIncludedColumnIndices 根据 ColumnBitmap1 获取被包含的列号列表
// ColumnBitmap1 中的每一位对应一列，1 表示包含，0 表示不包含
func getIncludedColumnIndices(totalColumns int, columnBitmap []byte) []int {
	var included []int

	for colIdx := 0; colIdx < totalColumns; colIdx++ {
		byteIdx := colIdx / 8
		bitIdx := colIdx % 8

		if byteIdx < len(columnBitmap) {
			// 检查对应的位是否为 1
			if (columnBitmap[byteIdx] & (1 << uint(bitIdx))) != 0 {
				included = append(included, colIdx)
			}
		}
	}

	return included
}
