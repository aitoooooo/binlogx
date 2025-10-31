package source

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	"github.com/aitoooooo/binlogx/pkg/models"
)

// MySQLSource 在线 MySQL 数据源
// 通过 MySQL Replication 协议读取二进制日志
type MySQLSource struct {
	dsn      string
	db       *sql.DB
	syncer   *replication.BinlogSyncer
	streamer *replication.BinlogStreamer
	eof      bool

	// 保存的表映射（用于事件转换）
	tableMap map[uint64]*replication.TableMapEvent
}

// NewMySQLSource 创建 MySQL 数据源
func NewMySQLSource(dsn string) *MySQLSource {
	return &MySQLSource{
		dsn:      dsn,
		tableMap: make(map[uint64]*replication.TableMapEvent),
	}
}

// Open 连接到 MySQL
func (ms *MySQLSource) Open(ctx context.Context) error {
	// 1. 测试标准 SQL 连接（用于列名缓存）
	db, err := sql.Open("mysql", ms.dsn)
	if err != nil {
		return fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping MySQL: %w", err)
	}
	ms.db = db

	// 2. 解析 DSN 以获取连接参数
	cfg, err := parseMySQLDSN(ms.dsn)
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}

	// 3. 创建 Binlog Syncer（用于读取二进制日志）
	syncerCfg := replication.BinlogSyncerConfig{
		ServerID: 1000,  // 需要唯一的 server ID
		Flavor:   "mysql",
		Host:     cfg["host"],
		Port:     parsePort(cfg["port"]),
		User:     cfg["user"],
		Password: cfg["password"],
	}

	syncer := replication.NewBinlogSyncer(syncerCfg)
	ms.syncer = syncer

	// 4. 获取当前 binlog 位置并开始同步
	var binlogFile string
	var binlogPos uint32

	// 尝试 SHOW MASTER STATUS（主库）
	row := db.QueryRowContext(ctx, "SHOW MASTER STATUS")
	err = row.Scan(&binlogFile, &binlogPos)
	if err != nil {
		// 如果是从库，尝试 SHOW SLAVE STATUS
		// SHOW SLAVE STATUS 返回很多列，我们需要用 QueryContext 而不是 QueryRowContext
		rows, err := db.QueryContext(ctx, "SHOW SLAVE STATUS")
		if err != nil {
			return fmt.Errorf("failed to get binlog position: %w", err)
		}
		defer rows.Close()

		if rows.Next() {
			// 获取所有列名
			cols, err := rows.Columns()
			if err != nil {
				return fmt.Errorf("failed to get columns: %w", err)
			}

			// 创建一个 interface{} 的切片来接收所有列
			values := make([]interface{}, len(cols))
			for i := range cols {
				values[i] = new(interface{})
			}

			if err := rows.Scan(values...); err != nil {
				return fmt.Errorf("failed to scan slave status: %w", err)
			}

			// 找到 Master_Log_File 和 Read_Master_Log_Pos 的索引
			var fileIdx, posIdx int
			for i, col := range cols {
				if col == "Master_Log_File" {
					fileIdx = i
				} else if col == "Read_Master_Log_Pos" {
					posIdx = i
				}
			}

			// 提取值
			if fileVal := values[fileIdx].(*interface{}); *fileVal != nil {
				// MySQL 驱动返回的可能是 []uint8 或 string
				switch v := (*fileVal).(type) {
				case string:
					binlogFile = v
				case []uint8:
					binlogFile = string(v)
				default:
					return fmt.Errorf("unexpected type for Master_Log_File: %T", v)
				}
			}
			if posVal := values[posIdx].(*interface{}); *posVal != nil {
				// MySQL 返回的位置可能是 int64、uint64、float64 或 []uint8
				switch v := (*posVal).(type) {
				case int64:
					binlogPos = uint32(v)
				case uint64:
					binlogPos = uint32(v)
				case float64:
					binlogPos = uint32(v)
				case []uint8:
					// 从字节数组转换为字符串，然后转为 uint32
					posStr := string(v)
					if p, err := strconv.ParseUint(posStr, 10, 32); err == nil {
						binlogPos = uint32(p)
					} else {
						return fmt.Errorf("failed to parse Read_Master_Log_Pos: %s", posStr)
					}
				default:
					return fmt.Errorf("unexpected type for Read_Master_Log_Pos: %T", v)
				}
			}
		} else {
			return fmt.Errorf("no slave status found, this is neither a master nor a slave")
		}
	}

	// 5. 启动 binlog 同步
	streamer, err := syncer.StartSync(mysql.Position{Name: binlogFile, Pos: binlogPos})
	if err != nil {
		return fmt.Errorf("failed to start binlog sync: %w", err)
	}
	ms.streamer = streamer
	ms.eof = false

	return nil
}

// Close 关闭连接
func (ms *MySQLSource) Close() error {
	var errs []string

	if ms.syncer != nil {
		ms.syncer.Close()
	}

	if ms.db != nil {
		if err := ms.db.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close MySQL connection: %s", strings.Join(errs, "; "))
	}

	return nil
}

// Read 读取下一个事件
func (ms *MySQLSource) Read() (*models.Event, error) {
	if ms.eof || ms.streamer == nil {
		return nil, fmt.Errorf("EOF")
	}

	// 从 binlog streamer 读取事件
	ctx := context.Background()
	ev, err := ms.streamer.GetEvent(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read binlog event: %w", err)
	}

	// 转换 go-mysql 的事件为项目中的 Event 模型
	event := ms.convertEvent(ev)
	if event == nil {
		// 某些事件类型我们不关心（如 TABLE_MAP），继续读下一个
		return ms.Read()
	}

	return event, nil
}

// HasMore 是否还有更多数据
func (ms *MySQLSource) HasMore() bool {
	return !ms.eof
}

// GetDB 获取数据库连接（用于列名缓存）
func (ms *MySQLSource) GetDB() *sql.DB {
	return ms.db
}

// convertEvent 将 go-mysql Event 转换为项目中的 Event 模型
func (ms *MySQLSource) convertEvent(ev *replication.BinlogEvent) *models.Event {
	if ev == nil {
		return nil
	}

	// 转换时间戳（Unix 时间戳 -> time.Time）
	timestamp := time.Unix(int64(ev.Header.Timestamp), 0)

	event := &models.Event{
		Timestamp: timestamp,
		EventType: ev.Header.EventType.String(),
		ServerID:  ev.Header.ServerID,
		LogPos:    ev.Header.LogPos,
	}

	// 根据事件类型提取具体信息
	switch e := ev.Event.(type) {
	case *replication.QueryEvent:
		// QUERY_EVENT: CREATE/DROP/ALTER 等 DDL 操作
		event.Database = string(e.Schema)
		event.Action = extractAction(string(e.Query))

	case *replication.RowsEvent:
		// ROWS_EVENT: INSERT/UPDATE/DELETE 等 DML 操作
		// 需要从之前保存的 TABLE_MAP 中获取表信息
		if tm, ok := ms.tableMap[e.TableID]; ok {
			event.Database = string(tm.Schema)
			event.Table = string(tm.Table)
		}

		// 根据事件类型判断操作
		switch ev.Header.EventType {
		case replication.WRITE_ROWS_EVENTv1, replication.WRITE_ROWS_EVENTv2:
			event.Action = "INSERT"
		case replication.UPDATE_ROWS_EVENTv1, replication.UPDATE_ROWS_EVENTv2:
			event.Action = "UPDATE"
		case replication.DELETE_ROWS_EVENTv1, replication.DELETE_ROWS_EVENTv2:
			event.Action = "DELETE"
		}

	case *replication.TableMapEvent:
		// TABLE_MAP_EVENT: 保存表信息，用于后续的 ROWS_EVENT
		ms.tableMap[e.TableID] = e
		return nil // 不返回这种事件

	case *replication.RotateEvent:
		// ROTATE_EVENT: binlog 轮转，不需要处理
		return nil

	case *replication.FormatDescriptionEvent:
		// FORMAT_DESCRIPTION_EVENT: binlog 格式描述
		return nil

	case *replication.XIDEvent:
		// XID_EVENT: 事务提交
		return nil

	default:
		// 其他事件类型暂不处理
		return nil
	}

	return event
}

// extractAction 从 SQL 语句中提取操作类型
func extractAction(query string) string {
	query = strings.TrimSpace(query)
	query = strings.ToUpper(query)

	if strings.HasPrefix(query, "INSERT") {
		return "INSERT"
	} else if strings.HasPrefix(query, "UPDATE") {
		return "UPDATE"
	} else if strings.HasPrefix(query, "DELETE") {
		return "DELETE"
	} else if strings.HasPrefix(query, "CREATE") {
		return "CREATE"
	} else if strings.HasPrefix(query, "DROP") {
		return "DROP"
	} else if strings.HasPrefix(query, "ALTER") {
		return "ALTER"
	} else if strings.HasPrefix(query, "BEGIN") {
		return "BEGIN"
	} else if strings.HasPrefix(query, "COMMIT") {
		return "COMMIT"
	}

	return "QUERY"
}

// parseMySQLDSN 解析 MySQL DSN
// 支持格式: user:password@tcp(host:port)/database?charset=utf8mb4
func parseMySQLDSN(dsn string) (map[string]string, error) {
	cfg := map[string]string{
		"user":     "root",
		"password": "",
		"host":     "127.0.0.1",
		"port":     "3306",
	}

	// 移除数据库和参数部分
	dsn = strings.Split(dsn, "?")[0]
	dsn = strings.Split(dsn, "/")[0]

	// 提取用户和密码
	if idx := strings.Index(dsn, "@"); idx != -1 {
		userPass := dsn[:idx]
		if pidx := strings.Index(userPass, ":"); pidx != -1 {
			cfg["user"] = userPass[:pidx]
			cfg["password"] = userPass[pidx+1:]
		} else {
			cfg["user"] = userPass
		}
		dsn = dsn[idx+1:]
	}

	// 提取 host 和 port
	// 格式: tcp(host:port) 或 host:port
	re := regexp.MustCompile(`(?:tcp\()?([^:)]+)(?::(\d+))?\)?`)
	matches := re.FindStringSubmatch(dsn)
	if len(matches) >= 2 {
		cfg["host"] = matches[1]
		if len(matches) >= 3 && matches[2] != "" {
			cfg["port"] = matches[2]
		}
	}

	return cfg, nil
}

// parsePort 解析端口号
func parsePort(portStr string) uint16 {
	port := 3306
	if p, err := strconv.Atoi(portStr); err == nil {
		port = p
	}
	return uint16(port)
}
