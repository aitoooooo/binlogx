package source

import (
	"context"

	"github.com/aitoooooo/binlogx/pkg/models"
)

// DataSource 数据源接口
type DataSource interface {
	// Open 打开数据源
	Open(ctx context.Context) error
	// Close 关闭数据源
	Close() error
	// Read 读取下一个事件
	Read() (*models.Event, error)
	// HasMore 是否还有更多数据
	HasMore() bool
}
