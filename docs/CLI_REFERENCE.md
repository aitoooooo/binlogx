# 命令行参考

## 全局选项

这些选项在所有命令中都可用。

### 数据源选项（必选其一）

#### `--source` string
离线 binlog 文件路径

```bash
binlogx stat --source /var/log/mysql/binlog.000001
```

#### `--db-connection` string
在线 MySQL 连接字符串

格式：`user:password@tcp(host:port)/dbname?charset=utf8mb4`

```bash
binlogx stat --db-connection "root:password@tcp(localhost:3306)/mydb?charset=utf8mb4"
```

在离线模式下同时指定此选项时，用于列名缓存。

### 时间过滤

#### `--start-time` string
开始时间，格式：`YYYY-MM-DD HH:MM:SS`

```bash
binlogx stat --source file.binlog --start-time "2024-01-01 10:00:00"
```

#### `--end-time` string
结束时间，格式：`YYYY-MM-DD HH:MM:SS`

```bash
binlogx stat --source file.binlog --end-time "2024-01-01 12:00:00"
```

### 操作类型过滤

#### `--action` strings (重复)
操作类型，可为 `INSERT`, `UPDATE`, `DELETE`

```bash
binlogx stat --source file.binlog --action INSERT --action UPDATE
```

### 分库表路由

#### `--db-regex` string
数据库名称正则表达式

```bash
# 匹配 db_0 到 db_9
binlogx stat --source file.binlog --db-regex "db_[0-9]"

# 匹配 db_001 到 db_999
binlogx stat --source file.binlog --db-regex "db_[0-9]{3}"
```

#### `--table-regex` string
表名称正则表达式

```bash
# 匹配 table_00 到 table_99
binlogx stat --source file.binlog --table-regex "table_[0-9]{2}"
```

#### `--include-db` strings (重复)
精确库名称列表

```bash
binlogx stat --source file.binlog --include-db db1 --include-db db2
```

#### `--include-table` strings (重复)
精确表名称列表

```bash
binlogx stat --source file.binlog --include-table users --include-table orders
```

### 并发和性能

#### `--workers` int
并发 worker 数量，默认为 CPU 核心数

```bash
# 使用 8 个 worker
binlogx stat --source file.binlog --workers 8
```

#### `--slow-threshold` string
慢方法阈值，默认 `1s`

格式：持续时间字符串 (如 `100ms`, `1.5s`)

```bash
# 记录耗时超过 500ms 的操作
binlogx stat --source file.binlog --slow-threshold 500ms
```

#### `--event-size-threshold` int
事件大小阈值（字节），默认 `0`（不检测）

```bash
# 警告超过 10MB 的事件
binlogx stat --source file.binlog --event-size-threshold 10485760
```

---

## 命令详解

### stat - 统计分布

显示 binlog 事件的统计信息

```bash
binlogx stat [options]
```

**选项**：

#### `--top` int, `-t`
仅显示前 N 条结果，默认 `0`（全部）

```bash
# 显示前 10 个最活跃的表
binlogx stat --source file.binlog --top 10
```

**输出**：
```
Total Events: 1234567

=== Database Distribution ===
  db1: 500000
  db2: 400000
  db3: 334567

=== Table Distribution ===
  db1.users: 250000
  db1.orders: 150000
  db2.logs: 400000
  ...

=== Action Distribution ===
  INSERT: 600000
  UPDATE: 450000
  DELETE: 184567
```

### parse - 交互式浏览

分页显示 binlog 事件详情

```bash
binlogx parse [options]
```

**选项**：

#### `--page-size` int, `-p`
每页显示的事件数，默认 `20`

```bash
binlogx parse --source file.binlog --page-size 50
```

**交互**：
- `n` - 下一页
- `q` - 退出

### sql - 生成前向 SQL

生成执行 binlog 中的更改所需的 SQL 语句

```bash
binlogx sql [options] > forward.sql
```

**输出**：
```sql
INSERT INTO `db1`.`users` (`id`, `name`, `email`) VALUES (1, 'John', 'john@example.com');
UPDATE `db1`.`users` SET `name`='Jane' WHERE `id`=1;
DELETE FROM `db1`.`users` WHERE `id`=2;
```

### rollback-sql - 生成回滚 SQL

生成撤销 binlog 中更改的 SQL 语句

```bash
binlogx rollback-sql [options] > rollback.sql
```

**选项**：

#### `--bulk` bool, `-b`
合并为批量 SQL，默认 `false`

```bash
# 逐条输出
binlogx rollback-sql --source file.binlog

# 批量输出
binlogx rollback-sql --source file.binlog --bulk
```

**回滚规则**：
- INSERT → DELETE（使用原始 VALUES）
- UPDATE → UPDATE（颠倒 SET 和 WHERE）
- DELETE → INSERT（使用原始 VALUES）

### export - 导出事件

导出 binlog 事件到多种格式

```bash
binlogx export --type <format> --output <path> [options]
```

**选项**：

#### `--type` string, `-t` (必填)
导出格式：`csv`, `sqlite`, `h2`, `hive`, `es`

#### `--output` string, `-o` (必填)
输出路径或连接字符串

**示例**：

```bash
# 导出为 CSV
binlogx export --source file.binlog \
  --type csv \
  --output ./export.csv

# 导出为 SQLite 数据库
binlogx export --source file.binlog \
  --type sqlite \
  --output ./binlog.db

# 导出到 Elasticsearch
binlogx export --source file.binlog \
  --type es \
  --output http://localhost:9200
```

### version - 版本信息

显示版本、构建时间和 Git 信息

```bash
binlogx version
```

**输出**：
```
binlogx version v1.0.0
Build time: 2024-01-01 10:00:00
Git commit: abc123def456
Git branch: main
```

---

## 使用场景

### 场景 1：数据恢复

```bash
# 1. 统计事件
binlogx stat --source file.binlog

# 2. 生成恢复 SQL
binlogx sql --source file.binlog > recover.sql

# 3. 执行 SQL
mysql -u root -p < recover.sql
```

### 场景 2：事件审计

```bash
# 导出为 CSV 进行分析
binlogx export --source file.binlog \
  --type csv \
  --output audit.csv

# 在 Excel 或数据库中分析
```

### 场景 3：特定时间范围的回滚

```bash
binlogx rollback-sql \
  --source file.binlog \
  --start-time "2024-01-01 10:00:00" \
  --end-time "2024-01-01 10:30:00" \
  --bulk > rollback.sql
```

### 场景 4：分库表处理

```bash
# 仅处理 db_0 到 db_9，table_00 到 table_99
binlogx stat --source file.binlog \
  --db-regex "db_[0-9]" \
  --table-regex "table_[0-9]{2}"
```

### 场景 5：大规模 binlog 处理

```bash
# 使用 16 个 worker，记录耗时操作
binlogx export --source file.binlog \
  --type sqlite \
  --output export.db \
  --workers 16 \
  --slow-threshold 500ms
```

---

## 错误处理

### 常见错误

#### "must specify either --source or --db-connection"
未指定数据源

```bash
# 正确
binlogx stat --source file.binlog
# 或
binlogx stat --db-connection "root:@tcp(localhost:3306)/db"
```

#### "unsupported export type"
导出格式不支持

```bash
# 支持的格式：csv, sqlite, h2, hive, es
binlogx export --type csv --output out.csv
```

#### "invalid start-time format"
时间格式错误

```bash
# 正确格式：YYYY-MM-DD HH:MM:SS
binlogx stat --source file.binlog --start-time "2024-01-01 10:00:00"
```
