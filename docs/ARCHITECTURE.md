# 项目架构设计

## 总体架构

```
┌─────────────────────────────────────────────────────┐
│           Cobra 命令行框架 (cmd/)                    │
│  ┌─────────┬────────┬────────┬─────────┬──────────┐ │
│  │stat│parse│sql│rollback│export│version │ │
│  └─────────┴────────┴────────┴─────────┴──────────┘ │
└────────────────────┬────────────────────────────────┘
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
    ┌─────────────┐      ┌──────────────┐
    │ FileSource  │      │MySQLSource   │
    │             │      │              │
    │binlog 文件  │      │在线数据库    │
    └─────────────┘      └──────────────┘
         │                       │
         └───────────┬───────────┘
                     ▼
        ┌──────────────────────┐
        │ 事件处理管道         │
        │ (EventProcessor)     │
        ├──────────────────────┤
        │ 生产者                │
        │ (1 goroutine)        │
        │ 读取 → 解析 → channel│
        └──────────┬───────────┘
                   │
            Channel (buffer=10000)
                   │
        ┌──────────┴──────────┐
        ▼          ▼          ▼
    ┌────────┐ ┌────────┐ ┌────────┐
    │Worker 0│ │Worker 1│ │Worker N│
    │ 过滤    │ │ 过滤    │ │ 过滤    │
    │ 转换    │ │ 转换    │ │ 转换    │
    │ 处理    │ │ 处理    │ │ 处理    │
    └────┬───┘ └────┬───┘ └────┬───┘
         │           │          │
         └───────────┼──────────┘
                     ▼
        ┌──────────────────────┐
        │  事件处理器链        │
        │ (EventHandlers)      │
        ├──────────────────────┤
        │ ├─ 统计处理器        │
        │ ├─ SQL 生成器        │
        │ ├─ CSV 导出器        │
        │ ├─ SQLite 导出器     │
        │ └─ ...              │
        └──────────────────────┘
```

## 模块职责

### 1. 数据源层 (pkg/source/)

**职责**：提供统一的数据读取接口

```go
interface DataSource {
    Open(ctx context.Context) error
    Read() (*Event, error)
    Close() error
    HasMore() bool
}
```

**实现**：
- `FileSource` - 离线 binlog 文件读取
- `MySQLSource` - 在线 MySQL 连接

### 2. 过滤层 (pkg/filter/)

**职责**：分库表路由和事件过滤

```
输入事件 → 检查 include-db/table → 检查 db-regex/table-regex → 输出/丢弃
```

**特性**：
- 精确匹配和正则匹配
- 确定性的 worker 路由（基于表和主键哈希）

### 3. 并发处理层 (pkg/processor/)

**职责**：生产者-消费者模式的并发事件处理

**设计**：
- 单个生产者 goroutine（保证顺序）
- N 个消费者 worker goroutine（并行处理）
- 有界 channel（防止内存爆炸）
- 背压机制（channel 满时阻塞）

**顺序保证**：
```
同一表同一主键的事件 → GetWorkerID() → 固定 Worker
确保因果一致性
```

### 4. 转换层 (pkg/util/sql_generator.go)

**职责**：将事件转换为 SQL 语句

```
INSERT event  → INSERT SQL
UPDATE event  → UPDATE SQL
DELETE event  → DELETE SQL
```

**回滚规则**：
```
INSERT → DELETE (使用 AfterValues)
UPDATE → UPDATE (颠倒 Before/After)
DELETE → INSERT (使用 BeforeValues)
```

### 5. 缓存层 (pkg/cache/)

**职责**：缓存表元数据以减少数据库查询

```
key: schema.table
value: TableMeta {Columns: [...]}
淘汰: LRU (上限 10,000)
```

**工作流**：
```
需要列名 → 检查缓存 → HIT: 返回 → MISS: 查询DB → 写入缓存 → 返回
```

### 6. 监控层 (pkg/monitor/)

**职责**：性能监控和告警

```
所有操作 → 记录耗时
耗时 > slow-threshold → 打印日志

事件字节数 > event-size-threshold → 打印告警日志
```

### 7. 命令层 (cmd/)

**职责**：CLI 接口和业务逻辑

```
stat     → 统计分布（数据库、表、操作类型）
parse    → 交互式分页浏览
sql      → 生成前向 SQL
rollback → 生成回滚 SQL
export   → 导出为多种格式
version  → 显示版本信息
```

## 数据流

### SQL 生成命令流程

```
1. main() 入口
2. cmd/root.go 初始化 Cobra
3. sqlCmd 触发
4. config.InitConfig() 解析全局参数
5. source.NewFileSource() 创建数据源
6. source.Open() 打开文件/数据库
7. filter.NewRouteFilter() 创建过滤器
8. processor.NewEventProcessor() 创建处理器
9. sqlHandler 创建 SQL 生成处理器
10. proc.Start() 启动处理
11. producer 读取事件，写入 channel
12. consumers 处理事件，调用 sqlHandler.Handle()
13. sqlHandler 生成 SQL 并输出
14. proc.Wait() 等待完成
15. proc.Flush() 刷新处理器
```

## 关键设计决策

### 1. 为什么使用有界 Channel？

防止内存爆炸。当处理大型 binlog 文件时，无界 channel 可能导致 OOM。

```go
eventChan: make(chan *Event, 10000)
```

### 2. 为什么要路由到固定 Worker？

保证因果一致性。同一条记录的多个版本必须按顺序处理，以保证 SQL 的正确性。

```go
workerID := GetWorkerID(table, primaryKey, workerCount)
```

### 3. 为什么使用接口而非具体实现？

支持扩展和测试。新增导出格式或数据源只需实现接口。

```go
type DataSource interface { ... }
type EventHandler interface { ... }
```

## 扩展点

### 添加新的导出格式

1. 实现 `EventHandler` 接口
2. 在 `cmd/export.go` 中注册
3. 添加相应的命令行标志

### 添加新的命令

1. 在 `cmd/` 下创建 `*.go` 文件
2. 实现 `*cobra.Command`
3. 在 `cmd/root.go` 的 `init()` 中注册

### 优化并发性能

1. 调整 `--workers` 参数
2. 优化 channel buffer 大小（当前 10,000）
3. 实现更复杂的事件分片策略

## 性能特性

### 内存管理

- 固定 goroutine 数（由 `--workers` 指定）
- 有界 channel（防止 OOM）
- 增量处理（不加载整个文件到内存）

### 并发策略

- 单生产者（保证顺序）
- 多消费者（利用多核）
- 哈希路由（保证因果一致性）

### 优化机会

- [ ] 使用对象池减少 GC 压力
- [ ] 实现优先级队列（重要表优先处理）
- [ ] 支持分布式处理（多机器）
