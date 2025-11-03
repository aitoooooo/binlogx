package source

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"
	_ "github.com/go-sql-driver/mysql"
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

	// 指定的起始位置（可选）
	startFile string
	startPos  uint32

	// 当前正在读取的 binlog 文件名
	currentLogName string
}

// NewMySQLSource 创建 MySQL 数据源
func NewMySQLSource(dsn string) *MySQLSource {
	return &MySQLSource{
		dsn:      dsn,
		tableMap: make(map[uint64]*replication.TableMapEvent),
	}
}

// SetStartPosition 设置起始位置（用于断点续看）
func (ms *MySQLSource) SetStartPosition(file string, pos uint32) {
	ms.startFile = file
	ms.startPos = pos
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
		ServerID: 1000, // 需要唯一的 server ID
		Flavor:   "mysql",
		Host:     cfg["host"],
		Port:     parsePort(cfg["port"]),
		User:     cfg["user"],
		Password: cfg["password"],
	}

	syncer := replication.NewBinlogSyncer(syncerCfg)
	ms.syncer = syncer

	// 4. 检查 binlog 是否启用
	var logBin string
	err = db.QueryRowContext(ctx, "SHOW VARIABLES LIKE 'log_bin'").Scan(new(string), &logBin)
	if err != nil || logBin != "ON" {
		return fmt.Errorf("binary logging is not enabled on this MySQL server. Please enable binlog in my.cnf with:\n  log-bin=mysql-bin\n  server-id=1")
	}

	// 5. 获取当前 binlog 位置并开始同步
	var binlogFile string
	var binlogPos uint32

	// 优先使用设置的起始位置（断点续看或命令行参数）
	if ms.startFile != "" && ms.startPos > 0 {
		binlogFile = ms.startFile
		binlogPos = ms.startPos
	} else {
		// 自动检测 binlog 位置
		// 尝试 SHOW MASTER STATUS（主库或独立实例）
		row := db.QueryRowContext(ctx, "SHOW MASTER STATUS")
		err = row.Scan(&binlogFile, &binlogPos)
		if err == sql.ErrNoRows || binlogFile == "" {
			// SHOW MASTER STATUS 返回空，可能是：
			// 1. 从库 - 尝试 SHOW REPLICA/SLAVE STATUS
			// 2. 主库但没有写入过数据 - 尝试 SHOW BINARY LOGS

			// 先尝试从库状态（MySQL 8.0.22+ 和 8.4+）
			rows, err := db.QueryContext(ctx, "SHOW REPLICA STATUS")
			if err != nil {
				// 如果新命令失败，尝试旧命令（兼容旧版本 MySQL）
				rows, err = db.QueryContext(ctx, "SHOW SLAVE STATUS")
			}

			if err == nil {
				defer rows.Close()
				if rows.Next() {
					// 这是从库，获取所有列名
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
						return fmt.Errorf("failed to scan replica/slave status: %w", err)
					}

					// 找到 binlog 文件和位置的列索引
					// MySQL 8.0.22+ 使用: Source_Log_File, Read_Source_Log_Pos
					// 旧版本使用: Master_Log_File, Read_Master_Log_Pos
					var fileIdx, posIdx int = -1, -1
					for i, col := range cols {
						// 新列名（MySQL 8.0.22+）
						if col == "Source_Log_File" {
							fileIdx = i
						} else if col == "Read_Source_Log_Pos" {
							posIdx = i
						}
						// 旧列名（兼容旧版本）
						if col == "Master_Log_File" && fileIdx == -1 {
							fileIdx = i
						}
						if col == "Read_Master_Log_Pos" && posIdx == -1 {
							posIdx = i
						}
					}

					if fileIdx == -1 || posIdx == -1 {
						return fmt.Errorf("could not find binlog file or position columns in replica/slave status")
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
							return fmt.Errorf("unexpected type for binlog file: %T", v)
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
								return fmt.Errorf("failed to parse binlog position: %s", posStr)
							}
						default:
							return fmt.Errorf("unexpected type for binlog position: %T", v)
						}
					}
				}
			}

			// 如果仍然没有获取到 binlog 文件，尝试使用 SHOW BINARY LOGS
			if binlogFile == "" {
				binlogRows, err := db.QueryContext(ctx, "SHOW BINARY LOGS")
				if err != nil {
					return fmt.Errorf("failed to get binary logs: %w", err)
				}
				defer binlogRows.Close()

				// 获取第一个（最旧的）binlog 文件
				if binlogRows.Next() {
					// SHOW BINARY LOGS 可能返回不同数量的列，取决于 MySQL 版本
					// 老版本：Log_name, File_size
					// 新版本：Log_name, File_size, Encrypted
					cols, err := binlogRows.Columns()
					if err != nil {
						return fmt.Errorf("failed to get binary logs columns: %w", err)
					}

					// 创建足够的变量来接收所有列
					values := make([]interface{}, len(cols))
					for i := range values {
						values[i] = new(interface{})
					}

					if err := binlogRows.Scan(values...); err != nil {
						return fmt.Errorf("failed to scan binary log: %w", err)
					}

					// 第一列总是 Log_name
					if logNameVal := values[0].(*interface{}); *logNameVal != nil {
						switch v := (*logNameVal).(type) {
						case string:
							binlogFile = v
						case []uint8:
							binlogFile = string(v)
						default:
							return fmt.Errorf("unexpected type for log name: %T", v)
						}
					}
					binlogPos = 4 // binlog 文件的起始位置总是 4
				} else {
					return fmt.Errorf("no binary logs found. Please check if binary logging is enabled and MySQL has write permissions")
				}
			}
		} else if err != nil {
			return fmt.Errorf("failed to execute SHOW MASTER STATUS: %w", err)
		}
	}

	// 6. 启动 binlog 同步
	streamer, err := syncer.StartSync(mysql.Position{Name: binlogFile, Pos: binlogPos})
	if err != nil {
		return fmt.Errorf("failed to start binlog sync: %w", err)
	}
	ms.streamer = streamer
	ms.eof = false
	ms.currentLogName = binlogFile // 设置当前 binlog 文件名

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
		LogName:   ms.currentLogName, // 填充当前 binlog 文件名
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
		tableMap, ok := ms.tableMap[e.TableID]
		if !ok {
			return event // 没有对应的 TABLE_MAP，无法处理
		}

		event.Database = string(tableMap.Schema)
		event.Table = string(tableMap.Table)

		// 根据事件类型判断操作
		switch ev.Header.EventType {
		case replication.WRITE_ROWS_EVENTv1, replication.WRITE_ROWS_EVENTv2:
			event.Action = "INSERT"
			// 对于 INSERT，Rows[0] 是插入的数据
			if len(e.Rows) > 0 {
				event.AfterValues = ms.rowToMap(e.Rows[0], e)
			}
		case replication.UPDATE_ROWS_EVENTv1, replication.UPDATE_ROWS_EVENTv2:
			event.Action = "UPDATE"
			// 对于 UPDATE，Rows 成对出现：[before, after, before, after, ...]
			if len(e.Rows) >= 2 {
				event.BeforeValues = ms.rowToMap(e.Rows[0], e)
				event.AfterValues = ms.rowToMap(e.Rows[1], e)
			}
		case replication.DELETE_ROWS_EVENTv1, replication.DELETE_ROWS_EVENTv2:
			event.Action = "DELETE"
			// 对于 DELETE，Rows[0] 是被删除的数据
			if len(e.Rows) > 0 {
				event.BeforeValues = ms.rowToMap(e.Rows[0], e)
			}
		}

	case *replication.TableMapEvent:
		// TABLE_MAP_EVENT: 保存表信息，用于后续的 ROWS_EVENT
		ms.tableMap[e.TableID] = e
		return nil // 不返回这种事件

	case *replication.RotateEvent:
		// ROTATE_EVENT: binlog 轮转，更新当前文件名
		ms.currentLogName = string(e.NextLogName)
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

// rowToMap 将行数据转换为 map
// 根据 ColumnBitmap1 确定哪些列被包含在行数据中
func (ms *MySQLSource) rowToMap(row []interface{}, rowsEvent *replication.RowsEvent) map[string]interface{} {
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
