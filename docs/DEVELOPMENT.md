# 开发指南

本文档说明如何进一步完善和扩展 binlogx。

## 项目架构

### 目录结构

```
binlogx/
├── cmd/                    # Cobra 命令定义
├── pkg/
│   ├── cache/             # 表元数据缓存
│   ├── config/            # 全局配置解析
│   ├── filter/            # 分库表路由过滤
│   ├── models/            # 数据模型
│   ├── monitor/           # 监控和告警
│   ├── processor/         # 事件处理和并发
│   ├── source/            # 数据源抽象
│   ├── util/              # 工具函数
│   └── version/           # 版本信息
├── main.go
├── Makefile
└── README.md
```

## 核心模块说明

### 1. 数据源模块 (pkg/source/)

**现状**：定义了接口，但 binlog 解析逻辑未实现

**待完善**：

```go
// pkg/source/file.go 中的 FileSource.Read()
// 需要实现真实的 binlog 解析逻辑
// 推荐使用 go-mysql 或类似库
```

建议使用以下库：
- [go-mysql](https://github.com/siddontang/go-mysql) - MySQL binlog 解析
- [go-mysql-elasticsearch](https://github.com/siddontang/go-mysql-elasticsearch) - 参考实现

### 2. SQL 生成模块 (pkg/util/sql_generator.go)

**现状**：基础 SQL 生成实现，需要增强

**待完善**：

1. **NULL 值处理**
   ```go
   // 当前实现：format Value() 函数
   // 待改进：更完善的 NULL 处理和类型转换
   ```

2. **特殊数据类型**
   ```go
   // 支持：JSON、Blob、Binary 等
   // 目前：简单转换
   ```

3. **转义和安全**
   ```go
   // 使用 sql 包的参数化查询
   // 或完善的 SQL 转义函数
   ```

### 3. 并发处理模块 (pkg/processor/processor.go)

**现状**：基本并发框架实现

**待完善**：

```go
// consumer() 函数中的 workerID 路由逻辑
// 目前：简单的哈希路由
// 待改进：支持更复杂的分片策略
```

### 4. 导出模块 (cmd/export_impl.go)

**现状**：CSV 和 SQLite 有基本实现，H2/Hive/ES 为占位符

**待完善**：

#### H2 导出
```go
// 使用 go-h2 驱动或 JDBC 协议
// 实现 H2Exporter 的 Handle() 和 Flush()
```

#### Hive 导出
```go
// 按日期分区生成 SQL
// 格式：/hive/binlog_export/date=YYYY-MM-DD/
// 可使用 Parquet 或 ORC 格式
```

#### Elasticsearch 导出
```go
// 使用 github.com/elastic/go-elasticsearch/v8
// 批量 Index 操作，支持自定义索引策略
// 例：binlog-2024-01-01, binlog-2024-01-02
```

### 5. 列名缓存 (pkg/cache/cache.go)

**现状**：基本 LRU 缓存实现

**待完善**：

```go
// 目前：简单清空 50% 缓存作为 LRU
// 待改进：真实 LRU 淘汰策略
// 建议使用：github.com/hashicorp/golang-lru
```

### 6. 监控模块 (pkg/monitor/monitor.go)

**现状**：基本日志输出

**待完善**：

```go
// 1. 结构化日志（JSON 格式）
// 2. 指标导出（Prometheus 格式）
// 3. 告警集成（Alertmanager/邮件）
```

## 关键任务清单

### 高优先级

- [ ] 实现 FileSource 中的 binlog 文件解析
  - 使用 go-mysql 库
  - 支持多个 binlog 文件顺序读取
  - 处理 binlog 格式错误

- [ ] 完善 SQL 生成
  - 支持所有 MySQL 数据类型
  - 正确处理转义和特殊字符
  - 参数化查询防止注入

- [ ] 实现 Elasticsearch 导出
  - 批量导入性能优化
  - 自定义索引映射

### 中优先级

- [ ] 完善 LRU 缓存实现
- [ ] 集成 Prometheus 监控指标
- [ ] 添加更多单元测试
- [ ] 支持 TLS/SSL 连接到 MySQL

### 低优先级

- [ ] H2 数据库导出
- [ ] Hive 集成
- [ ] 性能基准测试
- [ ] Docker 镜像

## 测试添加指南

### 添加新的单元测试

```go
// 在对应包下创建 *_test.go 文件
package mypackage

import "testing"

func TestMyFunction(t *testing.T) {
    // Arrange
    expected := "foo"

    // Act
    result := MyFunction()

    // Assert
    if result != expected {
        t.Errorf("Expected %s, got %s", expected, result)
    }
}
```

### 运行测试

```bash
# 运行单个包的测试
go test -v ./pkg/mypackage

# 运行所有测试
make test

# 查看覆盖率
make test
open coverage.html
```

## 构建和部署

### 本地开发

```bash
make build
./bin/binlogx --help
```

### 安装到系统

```bash
make install
binlogx version
```

### 跨平台编译

```bash
make cross-compile
ls -la bin/
```

## 常见问题

### Q: 如何调试 binlog 解析逻辑？

A: 使用 `-race` 标志运行测试以检测并发问题：
```bash
go test -race ./...
```

### Q: 如何提高并发性能？

A: 调整 `--workers` 参数：
```bash
binlogx stat --source file.binlog --workers 16
```

### Q: 如何调查慢操作？

A: 使用 `--slow-threshold` 参数：
```bash
binlogx stat --source file.binlog --slow-threshold 500ms
```

## 贡献指南

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交变更 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 参考资源

- [MySQL Binlog 格式](https://dev.mysql.com/doc/internals/en/binlog-event.html)
- [go-mysql 库](https://github.com/siddontang/go-mysql)
- [Cobra 框架](https://cobra.dev/)
- [Go 最佳实践](https://golang.org/doc/effective_go)
