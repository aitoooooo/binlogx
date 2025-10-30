# 快速开始指南

## 安装

### macOS (Apple Silicon)

```bash
make build
./bin/binlogx version
```

### Linux

```bash
make cross-compile
./bin/binlogx-linux-amd64 version
```

### Windows

```bash
make cross-compile
.\bin\binlogx-windows-amd64.exe version
```

## 基础使用

### 1. 查看 binlog 统计

```bash
./bin/binlogx stat --source /path/to/binlog.000001
```

### 2. 导出为 CSV

```bash
./bin/binlogx export --source /path/to/binlog.000001 \
  --type csv \
  --output ./binlog_export.csv
```

### 3. 生成前向 SQL

```bash
./bin/binlogx sql --source /path/to/binlog.000001 > forward.sql
```

### 4. 生成回滚 SQL

```bash
./bin/binlogx rollback-sql --source /path/to/binlog.000001 > rollback.sql
```

## 连接到在线 MySQL

```bash
# 连接到 MySQL 8.0
./bin/binlogx stat \
  --db-connection "user:password@tcp(localhost:3306)/dbname?charset=utf8mb4"
```

## 过滤示例

### 按时间过滤

```bash
./bin/binlogx stat --source /path/to/binlog.000001 \
  --start-time "2024-01-01 10:00:00" \
  --end-time "2024-01-01 12:00:00"
```

### 按操作类型过滤

```bash
./bin/binlogx stat --source /path/to/binlog.000001 \
  --action INSERT \
  --action UPDATE
```

### 按库表过滤

```bash
# 精确匹配
./bin/binlogx stat --source /path/to/binlog.000001 \
  --include-db mydb1 \
  --include-table users

# 正则匹配
./bin/binlogx stat --source /path/to/binlog.000001 \
  --db-regex "db_[0-9]" \
  --table-regex "table_[0-15]"
```

## 性能调优

```bash
# 使用更多 worker
./bin/binlogx stat --source /path/to/binlog.000001 --workers 16

# 记录慢操作
./bin/binlogx stat --source /path/to/binlog.000001 \
  --slow-threshold 500ms

# 检测大事件
./bin/binlogx stat --source /path/to/binlog.000001 \
  --event-size-threshold 10485760
```

## 故障排除

### 编译错误

确保 Go 版本 >= 1.25：
```bash
go version
```

### 缺少依赖

重新下载依赖：
```bash
go mod tidy
go mod download
```

### SQLite 编译问题（Linux）

```bash
# 需要 C 编译器
sudo apt-get install build-essential
```
