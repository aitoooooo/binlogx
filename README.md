# binlogx

高性能 MySQL binlog 处理工具，支持离线文件和在线数据库两种数据源，对 binlog 进行统计、解析、SQL 生成、回滚 SQL、多格式导出。

## 特性

- **多源数据支持**：离线 binlog 文件和在线 MySQL 数据库
- **6 个核心命令**：`stat`, `parse`, `sql`, `rollback-sql`, `export`, `version`
- **分库表范围表达式**：支持范围表达式的灵活路由
- **列名缓存系统**：智能缓存表元数据，减少数据库查询
- **并发处理**：生产者-消费者模型，可配置 worker 数量
- **监控能力**：慢方法监控和大事件预警
- **多格式导出**：CSV、SQLite、H2、Hive、Elasticsearch
- **SQL 生成**：自动生成前向和回滚 SQL

## 安装

### 从源代码构建

```bash
git clone https://github.com/aitoooooo/binlogx.git
cd binlogx
make build
```

### 安装到系统

```bash
make install
```

### 跨平台编译

```bash
make cross-compile
```

这将生成以下可执行文件：
- `binlogx-linux-amd64`
- `binlogx-linux-arm64`
- `binlogx-darwin-amd64`
- `binlogx-darwin-arm64`
- `binlogx-windows-amd64.exe`

## 使用示例

### 查看版本

```bash
binlogx version
```

### 统计事件分布

```bash
# 从离线文件统计
binlogx stat --source /path/to/binlog.000001

# 从在线数据库统计
binlogx stat --db-connection "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4"

# 按时间范围过滤
binlogx stat --source /path/to/binlog.000001 \
  --start-time "2024-01-01 10:00:00" \
  --end-time "2024-01-01 12:00:00"

# 按操作类型过滤并显示前 10 条
binlogx stat --source /path/to/binlog.000001 \
  --action INSERT --action UPDATE \
  --top 10
```

### 交互式查看事件详情

```bash
binlogx parse --source /path/to/binlog.000001
```

### 生成前向 SQL

```bash
binlogx sql --source /path/to/binlog.000001 > forward.sql
```

### 生成回滚 SQL

```bash
# 逐行输出回滚 SQL
binlogx rollback-sql --source /path/to/binlog.000001

# 合并为批量 SQL
binlogx rollback-sql --source /path/to/binlog.000001 --bulk
```

### 导出为 CSV

```bash
binlogx export --source /path/to/binlog.000001 --type csv --output /path/to/output/
```

### 导出为 SQLite

```bash
binlogx export --source /path/to/binlog.000001 --type sqlite --output /path/to/binlog.db
```

## 全局参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--source` | string | ① | 离线 binlog 文件路径 |
| `--db-connection` | string | ① | 在线 DSN `user:pass@tcp(host:port)/dbname?charset=utf8mb4` |
| `--start-time` | string | N | 开始时间 `YYYY-MM-DD HH:MM:SS` |
| `--end-time` | string | N | 结束时间 `YYYY-MM-DD HH:MM:SS` |
| `--action` | []string | N | 操作类型过滤（INSERT, UPDATE, DELETE） |
| `--slow-threshold` | duration | N | 慢方法阈值，默认 50ms |
| `--event-size-threshold` | int | N | 事件大小阈值（字节），默认 1024 |
| `--schema-table-regex` | []string | N | 分库表范围匹配，例 `db_[0-3].my_table_[0-99]` |
| `--workers` | int | N | worker 数量，默认 0=CPU 数 |

① 二选一：`--source` 和 `--db-connection` 必须指定其一

## 命令说明

### stat

统计 binlog 中的事件分布情况。

```bash
binlogx stat --source /path/to/binlog.000001 --top 20
```

输出：
- 总事件数
- 库分布
- 表分布
- 操作类型分布

### parse

交互式分页查看 binlog 事件详情。

```bash
binlogx parse --source /path/to/binlog.000001
```

按 `空格` 或 `Enter` 显示下一个事件，`q` 退出浏览。

### sql

生成前向 SQL 语句（从 binlog 恢复数据）。

```bash
binlogx sql --source /path/to/binlog.000001 > forward.sql
```

### rollback-sql

生成回滚 SQL 语句（undo 操作）。

```bash
# 单条输出
binlogx rollback-sql --source /path/to/binlog.000001

# 批量输出
binlogx rollback-sql --source /path/to/binlog.000001 --bulk
```

### export

导出 binlog 事件到多种格式。

```bash
# CSV
binlogx export --source /path/to/binlog.000001 --type csv --output ./export.csv

# SQLite
binlogx export --source /path/to/binlog.000001 --type sqlite --output ./binlog.db

# Elasticsearch
binlogx export --source /path/to/binlog.000001 --type es --output http://localhost:9200
```

支持的导出格式：
- `csv` - CSV 文件
- `sqlite` - SQLite 数据库
- `h2` - H2 数据库文件
- `hive` - Hive 分区表
- `es` - Elasticsearch

### version

显示版本信息、构建时间、Git 提交 ID 和分支。

```bash
binlogx version
```

## 分库表范围匹配

### 匹配语法

使用简化的**区间 + 通配符**语法：

- `*` - 匹配任意字母/数字/下划线（至少一个）
- `[a-b]` - 整数闭区间，展开为 `(a|a+1|...|b)`
- 其他字符原样匹配

### 使用示例

```bash
# 匹配 db_0 到 db_9 库的所有表
binlogx stat --source /path/to/binlog.000001 --schema-table-regex "db_[0-9].*"

# 匹配 db_0~db_9 库的 table_00~table_99 表
binlogx stat --source /path/to/binlog.000001 --schema-table-regex "db_[0-9].table_[0-99]"

# 匹配所有库的 users 表
binlogx stat --source /path/to/binlog.000001 --schema-table-regex "*.users"

# 使用多个匹配条件（OR 逻辑）
binlogx stat --source /path/to/binlog.000001 \
  --schema-table-regex "db_[0-3].*" \
  --schema-table-regex "prod.*"
```

### 常见模式

| 模式 | 说明 | 匹配示例 |
|------|------|---------|
| `db_[0-9].*` | db_0 到 db_9 的所有表 | db_0.users, db_5.orders |
| `db_[0-9].table_[0-99]` | 特定库表范围 | db_0.table_00, db_5.table_50 |
| `*.users` | 所有库的 users 表 | mydb.users, test.users |
| `[0-3].log*` | 0-3 库的 log 开头的表 | 0.logs, 2.log_events |

## 并发配置

```bash
# 使用 8 个 worker 并发处理
binlogx stat --source /path/to/binlog.000001 --workers 8

# 自动使用 CPU 数作为 worker 数（默认）
binlogx stat --source /path/to/binlog.000001
```

## 监控和告警

### 慢方法监控

```bash
# 记录耗时超过 2 秒的操作
binlogx stat --source /path/to/binlog.000001 --slow-threshold 2s
```

日志输出：
```
[SLOW] processBinlog took 2.5s, args: ...
```

### 大事件预警

```bash
# 记录超过 10MB 的单个事件
binlogx stat --source /path/to/binlog.000001 --event-size-threshold 10485760
```

日志输出：
```
[WARN] Large event detected: log-pos=1234567, type=QueryEvent, size=10485760 bytes
```

## 开发和测试

### 运行所有测试

```bash
make test
```

### 生成覆盖率报告

```bash
make test
open coverage.html
```

### 清理构建产物

```bash
make clean
```

## 项目结构

```
binlogx/
├── cmd/                    # 命令行命令定义
│   ├── root.go
│   ├── stat.go
│   ├── parse.go
│   ├── sql.go
│   ├── rollback_sql.go
│   ├── export.go
│   └── version.go
├── pkg/
│   ├── cache/             # 列名缓存
│   ├── config/            # 全局配置
│   ├── filter/            # 分库表路由过滤
│   ├── models/            # 数据模型
│   ├── monitor/           # 监控和告警
│   ├── processor/         # 事件处理器和并发模型
│   ├── source/            # 数据源接口和实现
│   ├── util/              # 工具函数
│   └── version/           # 版本信息
├── main.go                # 程序入口
├── go.mod
├── Makefile
└── README.md
```

## 核心实现细节

### 列名缓存

- **键**：`schema.table`
- **大小**：上限 10,000 表
- **淘汰**：LRU 策略

缓存在以下情况下会查询数据库：
- 离线/在线模式同时指定 `--db-connection`
- 未指定时自动降级为 `col_N` 格式

### 生产者-消费者模型

- **生产者**：单个 goroutine 顺序读取事件，写入有界 channel（缓冲 10,000 条）
- **消费者**：N 个 worker goroutine 并行处理事件
- **背压**：channel 满时生产者阻塞，防止内存暴涨
- **顺序保证**：同一表同一主键的事件路由到同一 worker

### 事件路由

使用 `table:primaryKeyHash % workerCount` 确保因果一致性。

## 许可证

MIT

## 文档

- [快速开始指南](docs/QUICKSTART.md) - 5 分钟快速上手
- [命令行参考](docs/CLI_REFERENCE.md) - 完整命令文档
- [项目架构](docs/ARCHITECTURE.md) - 设计和实现细节
- [开发指南](docs/DEVELOPMENT.md) - 扩展和贡献指南

## 贡献

欢迎提交 Issue 和 Pull Request！

## 相关资源

- [MySQL Binlog 文档](https://dev.mysql.com/doc/refman/8.0/en/binary-log.html)
- [Go MySQL 驱动](https://github.com/go-sql-driver/mysql)
- [Cobra 框架](https://github.com/spf13/cobra)
