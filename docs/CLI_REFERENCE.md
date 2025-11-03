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

**适用于**：离线文件和在线数据库

```bash
# 离线文件
binlogx stat --source file.binlog --start-time "2024-01-01 10:00:00"

# 在线数据库
binlogx sql --db-connection "user:pass@tcp(host:port)/" --start-time "2024-01-01 10:00:00"
```

#### `--end-time` string
结束时间，格式：`YYYY-MM-DD HH:MM:SS`

**适用于**：离线文件和在线数据库

**重要**：对于在线数据库，指定 `--end-time` 后，程序会在读取到超过结束时间的事件时自动停止并退出，避免需要手动按 Ctrl+C 中断。

```bash
# 离线文件：只处理指定时间范围内的事件
binlogx stat --source file.binlog --end-time "2024-01-01 12:00:00"

# 在线数据库：自动停止在结束时间
binlogx sql --db-connection "user:pass@tcp(host:port)/" \
    --end-time "2024-01-01 12:00:00" > output.sql

# 指定完整时间范围
binlogx export --db-connection "user:pass@tcp(host:port)/" \
    --start-time "2024-01-01 10:00:00" \
    --end-time "2024-01-01 12:00:00" \
    --type csv --output export.csv
```

**使用场景**：
- 离线文件：过滤特定时间范围的事件
- 在线数据库：导出历史数据到特定时间点，程序自动停止

### 操作类型过滤

#### `--action` strings (重复)
操作类型，可为 `INSERT`, `UPDATE`, `DELETE`

```bash
binlogx stat --source file.binlog --action INSERT --action UPDATE
```

### 分库表范围匹配

#### `--schema-table-regex` strings (重复)
使用简化的**区间 + 通配符**语法对数据库和表进行灵活匹配

**语法说明**：
- `*` - 匹配任意字母/数字/下划线（至少一个）
- `[a-b]` - 整数闭区间匹配，展开为 `(a|a+1|...|b)`
- 其他字符原样匹配

```bash
# 匹配 db_0 到 db_9 库的所有表
binlogx stat --source file.binlog --schema-table-regex "db_[0-9].*"

# 匹配 db_0~db_9 库的 table_00~table_99 表
binlogx stat --source file.binlog --schema-table-regex "db_[0-9].table_[0-99]"

# 匹配所有库的 users 表
binlogx stat --source file.binlog --schema-table-regex "*.users"

# 使用多个匹配条件（所有条件 OR 逻辑）
binlogx stat --source file.binlog \
  --schema-table-regex "db_[0-3].*" \
  --schema-table-regex "prod.*"
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

逐个显示 binlog 事件详情，支持交互式控制和断点续看

```bash
binlogx parse [options]
```

**局部参数**：

#### `--start-log-file` string
起始 binlog 文件名（例如 mysql-bin.000001）

**仅用于在线数据库**（`--db-connection`）

```bash
# 从指定文件开始读取
binlogx parse --db-connection "user:pass@tcp(host:port)/" \
    --start-log-file mysql-bin.000002
```

#### `--start-log-pos` uint32
起始 binlog 位置

**仅用于在线数据库**（`--db-connection`），必须与 `--start-log-file` 一起使用

```bash
# 从指定位置开始读取
binlogx parse --db-connection "user:pass@tcp(host:port)/" \
    --start-log-file mysql-bin.000001 \
    --start-log-pos 1234
```

**特性**：
- 流式输出：事件一经解析即刻显示，无需等待完整文件处理
- 交互式分页：每个事件显示后等待用户操作
- 断点续看：自动保存和恢复浏览位置
- 列名映射：支持离线模式+在线数据库组合获取真实列名
- 完整事件信息：JSON 格式输出，包含所有元数据和生成的 SQL

**列名映射功能**：

默认情况下，binlog 中的行数据使用占位符列名（如 `col_0`, `col_1`）。要显示真实列名，需要提供数据库连接：

```bash
# 离线文件 + 数据库连接 = 真实列名
binlogx parse --source /path/to/binlog.000001 \
    --db-connection "user:pass@tcp(host:port)/"

# 纯离线模式 = 占位符列名
binlogx parse --source /path/to/binlog.000001

# 在线数据库 = 自动获取真实列名
binlogx parse --db-connection "user:pass@tcp(host:port)/"
```

**交互方式**：
- 按 `空格` 或 `Enter` 显示下一个事件
- 按 `q` 退出浏览

**断点续看机制**：

当浏览完成后退出（按 `q`），系统会自动保存当前位置到 `~/.binlogx/checkpoints/` 目录。下次运行同样的 parse 命令时：
1. 系统检测到已保存的断点
2. 提示用户是否从上次位置继续（`y/n，默认y`）
3. 若选择 `y`，从上次位置之后的事件开始显示
4. 若选择 `n`，从头开始读取并清除旧断点

**示例**：

```bash
# 第一次浏览，查看一部分事件后按 q 退出
$ binlogx parse --source /path/to/binlog.000001
[Event 1]
...
[Event 100]
Press SPACE/Enter for next, 'q' to quit: q
正在退出...
断点已保存: mysql-bin.000786:50000
总共显示事件数: 100

# 第二次运行会自动提示加载断点
$ binlogx parse --source /path/to/binlog.000001
找到上次的断点位置:
  文件: mysql-bin.000786
  位置: 50000
  时间: 2025-11-03 11:00:00

是否从断点继续？(y/n，默认y): y
从断点继续: mysql-bin.000786:50000

[Event 101]
...
```

**断点文件位置**：
- 离线文件模式：`~/.binlogx/checkpoints/file_<hash>.json`
- 在线 MySQL 模式：`~/.binlogx/checkpoints/mysql_<hash>.json`

其中 `<hash>` 是数据源路径或 DSN 的哈希值。

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

#### `--action` string, `-a` (可选)
要导出的事件类型，以逗号分隔，默认：`INSERT,UPDATE,DELETE`

```bash
# 仅导出 INSERT 和 UPDATE
binlogx export --source file.binlog \
  --type sqlite \
  --output export.db \
  --action "INSERT,UPDATE"
```

#### `--batch-size` int, `-b` (可选)
批处理大小，默认：`1000`。更大的值提高速率但占用更多内存；更小的值降低内存占用但性能较低。

```bash
# 使用较大的批处理大小以获得更好的性能
binlogx export --source file.binlog \
  --type sqlite \
  --output export.db \
  --batch-size 5000

# 使用较小的批处理大小以降低内存占用
binlogx export --source file.binlog \
  --type sqlite \
  --output export.db \
  --batch-size 500
```

#### `--estimate-total` bool, `-e` (可选)
在导出前快速扫描统计总事件数，以便显示更准确的进度百分比，默认：`false`

```bash
# 启用总事件数预估
binlogx export --source file.binlog \
  --type sqlite \
  --output export.db \
  --estimate-total
```

**示例**：

```bash
# 导出为 CSV（使用默认参数）
binlogx export --source file.binlog \
  --type csv \
  --output ./export.csv

# 导出为 SQLite 数据库（自定义批处理大小）
binlogx export --source file.binlog \
  --type sqlite \
  --output ./binlog.db \
  --batch-size 2000

# 导出到 Elasticsearch（显示精确进度）
binlogx export --source file.binlog \
  --type es \
  --output http://localhost:9200 \
  --estimate-total

# 导出特定操作类型（仅 INSERT）
binlogx export --source file.binlog \
  --type sqlite \
  --output ./inserts.db \
  --action INSERT \
  --batch-size 3000
```

**性能优化建议**：

为了获得最佳性能，建议根据硬件配置调整参数：
- **高端服务器**（32+ 核心）：`--batch-size 5000` 和 `--workers 32`
- **中端服务器**（8-16 核心）：`--batch-size 2000` 和 `--workers 8`
- **低端服务器**（2-4 核心）：`--batch-size 500` 和 `--workers 4`

**输出示例**：
```
[完成] 总耗时: 1m40s

[统计] 进度: 100.0% (188049/188049) | 已导出: 188049 | 平均速率: 1877.4 events/sec

[性能指标]
  总耗时: 1m40s (1.67 分钟)
  处理事件: 188049 个
  导出事件: 188049 个
  平均速率: 1877.40 events/sec
  导出成功率: 100.00%
  导出速率: 1877.40 events/sec
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
# 匹配 db_0 到 db_9 库的所有表
binlogx stat --source file.binlog \
  --schema-table-regex "db_[0-9].*"

# 匹配 db_0~db_9 库的 table_00~table_99 表
binlogx stat --source file.binlog \
  --schema-table-regex "db_[0-9].table_[0-99]"
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
