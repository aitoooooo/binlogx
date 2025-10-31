# binlogx 架构设计文档

## 1. 项目概述

**binlogx** 是一个高性能的 MySQL binlog 处理工具，用 Go 语言编写，支持离线文件和在线数据库两种数据源。核心功能包括事件统计、交互式解析、SQL 生成、回滚 SQL、多格式导出和性能监控。

### 核心特性
- **双源支持**：离线 binlog 文件和在线 MySQL 数据库
- **6 个核心命令**：stat、parse、sql、rollback-sql、export、version
- **智能缓存**：列名元数据缓存，支持 LRU 淘汰和后台清理
- **高效并发**：生产者-消费者模型 + 分片锁架构
- **灵活路由**：区间 + 通配符范围匹配
- **性能监控**：慢方法监控、大事件预警、实时进度跟踪

---

## 2. 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                    用户界面层 (CLI)                          │
│  ┌──────────┬────────┬────────┬──────────┬────────┬────────┐ │
│  │  stat    │ parse  │  sql   │rollback  │export  │version │ │
│  │ 统计分布  │交互解析│前向SQL  │回滚SQL   │多格式  │版本信息│ │
│  └──────────┴────────┴────────┴──────────┴────────┴────────┘ │
└──────────────────────┬──────────────────────────────────────┘
                       │
        ┌──────────────┴──────────────┐
        ▼                              ▼
   ┌──────────────┐          ┌─────────────────┐
   │ 数据源层      │          │ 配置管理层      │
   ├──────────────┤          ├─────────────────┤
   │FileSource    │          │全局参数解析      │
   │MySQLSource   │          │命令行标志定义    │
   │Open/Read     │          │默认值配置        │
   │Close         │          └─────────────────┘
   └──────┬───────┘
          │
          ▼
   ┌─────────────────────────────────────────┐
   │     事件处理管道 (EventProcessor)       │
   │  ┌──────────────────────────────────┐   │
   │  │ 生产者 (1 goroutine)             │   │
   │  │ ├─ Read binlog/MySQL             │   │
   │  │ ├─ Parse event                   │   │
   │  │ └─ Write to channel              │   │
   │  └──────────────┬───────────────────┘   │
   │                 │                        │
   │        Channel (buffer=10000)            │
   │                 │                        │
   │  ┌──────────────┴───────────────────┐   │
   │  │ 消费者 (N workers)               │   │
   │  │ ├─ Worker 0: 过滤 → 转换 → 处理  │   │
   │  │ ├─ Worker 1: 过滤 → 转换 → 处理  │   │
   │  │ └─ Worker N: 过滤 → 转换 → 处理  │   │
   │  └──────────────┬───────────────────┘   │
   └─────────────────┼──────────────────────┘
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
   ┌─────────┐ ┌──────────┐ ┌──────────┐
   │ 过滤器   │ │ 缓存层   │ │ 监控层   │
   │ Filter  │ │ Cache    │ │ Monitor  │
   └────┬────┘ └────┬─────┘ └────┬─────┘
        │           │            │
        └───────────┼────────────┘
                    │
        ┌───────────┴───────────┐
        ▼                       ▼
   ┌───────────────┐    ┌──────────────────┐
   │ 业务处理器     │    │ 工具组件         │
   ├───────────────┤    ├──────────────────┤
   │StatHandler    │    │SQLGenerator      │
   │ParseHandler   │    │RangeMatcher      │
   │SQLHandler     │    │ShardedLock       │
   │ExportHandler  │    │DataTypeConverter │
   │RollbackHandler│    │VersionInfo      │
   └───────────────┘    └──────────────────┘
```

---

## 3. 核心模块详解

### 3.1 数据源层 (pkg/source/)

**职责**：提供统一的数据读取接口，支持多种数据源

```go
// 统一的数据源接口
type DataSource interface {
    Open(ctx context.Context) error
    Read() (*Event, error)
    Close() error
    HasMore() bool
}
```

**实现**：
- **FileSource** - 离线 binlog 文件读取
  - 打开本地 binlog 文件
  - 逐个解析 binary log event
  - 流式返回事件，支持中断

- **MySQLSource** - 在线 MySQL 连接
  - 连接到 MySQL 实例
  - 获取二进制日志流
  - 支持实时读取

**关键特性**：
- ✅ 上下文支持（可中断）
- ✅ 错误恢复
- ✅ 流式处理（不加载整个文件到内存）

---

### 3.2 配置管理层 (pkg/config/)

**职责**：集中管理所有全局配置和命令行参数

**核心结构**：
```go
type GlobalConfig struct {
    Source             string              // binlog 文件路径
    DBConnection       string              // MySQL 连接 DSN
    StartTime          time.Time           // 时间范围
    EndTime            time.Time
    Action             []string            // 操作类型过滤 (INSERT/UPDATE/DELETE)
    SlowThreshold      time.Duration       // 慢方法阈值
    EventSizeThreshold int64               // 大事件阈值
    SchemaTableRegex   []string            // 分库表范围匹配
    Workers            int                 // 并发 worker 数
}
```

**全局单例**：
- `GlobalConfig` - 所有模块共享的配置
- `GlobalMonitor` - 全局监控对象

---

### 3.3 事件处理管道 (pkg/processor/)

**架构设计**：生产者-消费者模式，实现高效并发处理

#### 生产者 (1 goroutine)
```
Read Event → Parse Event → Enqueue to Channel
```
- 单线程读取，保证顺序性
- 有界 channel（缓冲 10,000 条）防止 OOM
- 顺序读取 → 顺序解析 → 顺序入队

#### 消费者 (N workers)
```
Dequeue Event → Route to Worker → Process & Handle
```
- N 个 worker 并行处理
- 基于表名和主键哈希路由（保证同一条记录的事件顺序）
- 支持可配置的 worker 数量

#### 背压机制
```
Channel 满 (10,000 events) → Producer 阻塞 → Memory 稳定
```

**处理流程**：
1. Producer 读取原始事件
2. 事件写入有界 channel
3. N 个 Worker 并行消费
4. 每个 Worker 调用处理器链 (Handler Chain)
5. 所有处理器完成后，Worker 继续消费

---

### 3.4 过滤层 (pkg/filter/)

**职责**：分库表范围匹配和事件路由

**匹配算法**：简化的区间 + 通配符语法（RangeMatcher）

```
输入模式 → 解析范围 → 生成正则表达式 → 匹配事件
```

**语法说明**：
- `*` - 匹配任意字母/数字/下划线（至少一个）
- `[a-b]` - 整数闭区间，展开为 `(a|a+1|...|b)`
- 其他字符原样匹配

**示例**：
```
模式: db_[0-9].*
展开: db_(0|1|2|...|9)[A-Za-z0-9_]+
匹配: db_0.users, db_5.orders, db_9.logs
```

**Worker 路由**：
```go
// 确保同一条记录的多个版本路由到同一 worker
workerID := hash(table + ":" + primaryKey) % workerCount
```

**意义**：因果一致性 - 同一条记录的 INSERT/UPDATE/UPDATE/DELETE 必须按顺序处理

---

### 3.5 缓存层 (pkg/cache/)

**职责**：缓存表元数据（列名、数据类型），减少数据库查询

**架构**：

```
请求列名
    ↓
检查本地缓存 (notFoundCache)
    ↓
HIT: 返回错误标记 (避免重复查询)
MISS:
    ↓
使用 singleflight 发起一次 DB 查询
    ↓
多个并发请求共享同一个 DB 查询结果
    ↓
写入缓存
    ↓
返回结果
```

**核心优化**：

1. **双重 Lock 合并** - 减少 RWLock 竞争
   ```go
   // 之前: 两次 RLock 获取
   // 现在: 合并为一次
   mu.RLock()
   notFound := rf.notFoundCache[key]
   cached := rf.cache[key]
   mu.RUnlock()
   ```

2. **Singleflight 模式** - 防止并发重复查询
   ```go
   // 相同的表，多个 goroutine 发起查询
   // 只有第一个发起 DB 查询，其他 goroutine 等待结果
   ```

3. **后台清理机制** - 防止内存泄漏
   ```go
   // 每 30 秒清理一次过期的 notFoundCache 条目
   // 定期检查并回收
   ```

4. **LRU 淘汰策略** - 上限 10,000 表
   ```go
   // 缓存满时，清空并重新开始
   // 每次命中时更新访问时间
   ```

**数据结构**：
```go
type MetaCache struct {
    cache          map[string]*TableMeta      // 成功缓存
    notFoundCache  map[string]time.Time       // 负缓存（不存在）
    mu             sync.RWMutex               // 并发控制
    maxSize        int                        // 上限（10,000）
    cleanupTicker  *time.Ticker               // 后台清理
}
```

---

### 3.6 监控层 (pkg/monitor/)

**职责**：性能监控和告警

**监控指标**：

1. **慢方法监控**
   - 方法名、入参、执行耗时
   - 阈值：`--slow-threshold`（默认 50ms）

2. **大事件预警**
   - log-pos、事件类型、大小
   - 阈值：`--event-size-threshold`（默认 1024 字节）

3. **进度跟踪**（export 命令）
   - 实时进度百分比
   - 预估剩余时间 (ETA)
   - 处理速率 (events/sec)

---

### 3.7 业务处理器 (cmd/)

**设计模式**：Handler Chain（处理器链）

#### 各命令的处理器

| 命令 | 处理器 | 职责 |
|------|--------|------|
| stat | StatHandler | 统计事件分布（库、表、操作类型） |
| parse | ParseHandler | 解析并显示事件详情（JSON 格式） |
| sql | SQLHandler | 生成前向 SQL 语句 |
| rollback-sql | RollbackHandler | 生成回滚 SQL 语句 |
| export | ExportHandler | 导出到 CSV/SQLite/H2/Hive/ES |

#### 处理器接口
```go
type EventHandler interface {
    Handle(event *Event) error
    Flush() error
}
```

#### 导出器详解（export 命令）

**支持的导出格式**：
1. **CSV** - CSV 文件
   - 最小化锁（仅在 writer.Write 时持锁）
   - CPU 密集操作（列名映射、SQL 生成）在锁外

2. **SQLite** - SQLite 数据库（推荐）
   - 应用分片锁（16 个分片）
   - 按 database.table 分片，实现并行写入
   - 批处理写入（可配置大小）
   - 性能提升：+30-50%（多表并行）

3. **H2** - H2 数据库（占位符）
4. **Hive** - Hive 分区表（占位符）
5. **Elasticsearch** - ES 索引（占位符）

**批处理优化**：
```
事件 1 → Buffer (batchSize=1000)
事件 2 → Buffer
...
事件 1000 → Buffer FULL → Flush to DB
事件 1001 → New Buffer
```

**分片锁设计**（SQLiteExporter）：
```go
// 16 个分片锁
shardLock := NewShardedLock(16)

// 基于表名生成分片 key
shardKey := event.Database + "." + event.Table
mu, shardIdx := shardLock.GetShard(shardKey)

// 不同表的操作可以并行进行
mu.Lock()
buffer[shardIdx] = append(buffer[shardIdx], event)
mu.Unlock()
```

**优势**：
- 不同表的批处理不会相互阻塞
- 避免全局锁导致的串行化
- 并发度 ↑ 16 倍

---

### 3.8 工具组件 (pkg/util/)

#### 1. SQLGenerator - SQL 生成
- **功能**：将 binlog 事件转换为 SQL 语句
- **支持**：INSERT、UPDATE、DELETE、前向、回滚
- **特性**：
  - 数据类型自动转换（DATETIME、JSON 等）
  - NULL 值处理
  - 特殊字符转义

#### 2. RangeMatcher - 范围匹配
- **功能**：将简化语法转换为正则表达式
- **支持**：通配符 (*) 和整数区间 ([a-b])
- **用途**：分库表过滤

#### 3. ShardedLock - 分片锁
- **功能**：降低锁竞争
- **实现**：哈希一致性分片
- **用途**：并发导出性能优化

#### 4. DataTypeConverter - 数据类型转换
- **功能**：binlog 二进制数据 → 人类可读格式
- **支持**：
  - 数值类型：INT, BIGINT, FLOAT 等
  - 字符串：VARCHAR, TEXT 等
  - 时间：DATETIME, TIMESTAMP 等
  - JSON 对象

---

## 4. 核心流程详解

### 4.1 导出流程 (export 命令)

```
用户执行: binlogx export --source file.bin --type sqlite --batch-size 2000

├─ 解析配置参数
│  ├─ source: file.bin
│  ├─ type: sqlite
│  ├─ batch-size: 2000
│  └─ workers: CPU count (默认)
│
├─ 创建数据源 (FileSource)
│  └─ 打开 binlog 文件
│
├─ 创建过滤器 (RouteFilter)
│  └─ 编译范围匹配规则
│
├─ 创建缓存 (MetaCache)
│  └─ 初始化元数据缓存
│
├─ 预扫描 (可选：--estimate-total)
│  ├─ 快速扫描文件计算总事件数
│  └─ 用于进度百分比显示
│
├─ 创建导出处理器
│  └─ newSQLiteExporter(output, batch-size=2000)
│
├─ 启动处理器
│  ├─ 生产者 goroutine: 读取事件 → 入队
│  ├─ N 个消费者 goroutine: 出队 → 过滤 → 处理
│  └─ 进度跟踪 goroutine: 每分钟输出进度
│
└─ 等待完成
   ├─ 所有事件处理完毕
   ├─ 处理器 Flush (刷新缓冲区)
   ├─ 输出性能统计
   └─ 关闭资源
```

**关键优化点**：
1. ✅ 预扫描计算总数（可选）→ 准确的进度
2. ✅ 分片锁 + 批处理 → 高吞吐量
3. ✅ CPU 密集操作在锁外 → 减少锁持有时间
4. ✅ 实时进度跟踪 → 用户感知

---

### 4.2 并发处理详解

```
生产者 (1 goroutine)
├─ Open FileSource
├─ for每个event {
│   ├─ Read() 获取二进制数据
│   ├─ Parse() 解析为 Event
│   ├─ eventChan <- Event (阻塞if满)
│   └─ }
└─ close(eventChan)

Channel (有界, size=10000)
├─ 缓冲事件
├─ 背压: 满时生产者阻塞
└─ 背压: 空时消费者等待

消费者 (N workers, N=CPU count)
├─ Worker 0: for event := range eventChan {
│   ├─ RouteFilter.Match(event) 检查是否匹配
│   ├─ if匹配 {
│   │   ├─ MapColumnNames(event) CPU密集
│   │   ├─ GenerateSQL(event) CPU密集
│   │   ├─ ExportHandler.Handle(event) 获取锁
│   │   └─ }
│   └─ }
├─ Worker 1: ...
└─ Worker N: ...

Worker 路由保证
├─ 同一表同一主键 → 固定 Worker
├─ 原因: 因果一致性
└─ 哈希: workerID = hash(table + pk) % N
```

**并发优化**：
- 生产者单线程 → 保持顺序
- 消费者多线程 → 利用多核
- 哈希路由 → 同记录顺序 + 不同记录并行

---

## 5. 性能优化

### 5.1 缓存优化

| 优化项 | 手段 | 效果 |
|--------|------|------|
| 双重 lock | 合并两次 RWLock 为一次 | -15~20% |
| Singleflight | 去重并发 DB 查询 | -30~40%（并发场景）|
| 后台清理 | 定期清理过期条目 | 稳定内存 |
| LRU 淘汰 | 上限 10,000 表 | 可控的内存占用 |

### 5.2 导出优化

| 优化项 | 手段 | 效果 |
|--------|------|------|
| CPU 密集操作出锁 | 列名映射、SQL生成在锁外 | -20~30% |
| 分片锁 (SQLite) | 16 个分片 | -30~50%（多表）|
| 批处理 | 可配置 batch-size | ↑ 吞吐量 |
| 无并发导出 | CSV 串行写入（设计合理）| N/A |

### 5.3 总体性能提升预期

```
优化前: ~1000 events/sec
优化后: ~1500-2000+ events/sec（取决于硬件和操作类型）

预期提升: +50-100%
```

**影响因素**：
- CPU 核心数（worker 数）
- 磁盘 I/O 速度（导出性能）
- binlog 事件大小分布
- 表的数量（分片效果）

---

## 6. 设计决策

### 6.1 为什么使用生产者-消费者模式？

**对比方案**：
| 方案 | 优点 | 缺点 |
|------|------|------|
| 生产者-消费者 | 解耦、并发、背压 | 略复杂 |
| 单线程流水线 | 简单 | 无法并行处理 |
| 无缓冲 channel | 内存小 | 生产者频繁阻塞 |

**选择理由**：
- ✅ 自动背压（缓冲 10,000）防止 OOM
- ✅ 单线程读取保证顺序
- ✅ 多线程处理利用多核
- ✅ 哈希路由保证因果一致性

### 6.2 为什么分库表参数用范围匹配而非正则？

**对比方案**：
| 方案 | 语法 | 学习成本 | 性能 |
|------|------|---------|------|
| 范围匹配（当前） | `db_[0-9].*` | 低 | 快 |
| Go 正则表达式 | `db_[0-9].*` | 中 | 快 |
| PCRE 正则 | `db_[0-9]{1,10}` | 高 | 慢 |

**选择理由**：
- ✅ 简化语法，用户友好
- ✅ 专为数字范围设计（`[a-b]` 展开）
- ✅ 实现简单，性能稳定
- ✅ 足以覆盖 99% 的使用场景

### 6.3 为什么 CSV 不用分片锁？

**分析**：
```
CSV 导出特点:
├─ 单个文件（不支持并发写入）
├─ 必须串行化写入
└─ 分片锁无法加速

CSV 当前设计:
├─ 仅在 writer.Write() 时持锁（~0.1ms）
├─ CPU 密集操作（列名映射、SQL生成）在锁外
└─ 已经最优化

结论: 分片锁无益，保持简单
```

### 6.4 为什么支持离线 + 在线两种源？

**需求**：
- 离线：开发、测试、大文件处理（无需数据库）
- 在线：实时监控、线上分析（需要列名缓存）

**实现**：
- 统一的 DataSource 接口
- 两种实现各自独立
- 配置层选择激活

---

## 7. 扩展性设计

### 7.1 添加新的导出格式

**步骤**：
1. 在 `cmd/export_impl.go` 中实现 `EventHandler` 接口
   ```go
   type NewExporter struct {
       // 字段
   }

   func (ne *NewExporter) Handle(event *Event) error {
       // 导出逻辑
   }

   func (ne *NewExporter) Flush() error {
       // 刷新缓冲
   }
   ```

2. 在 `cmd/export.go` 的 `RunE` 中注册
   ```go
   case "newformat":
       handler, err := newNewExporter(output, helper, actions, batchSize)
   ```

3. 更新 `docs/CLI_REFERENCE.md` 文档

### 7.2 添加新的命令

**步骤**：
1. 在 `cmd/` 目录下创建 `newcmd.go`
   ```go
   var newCmd = &cobra.Command{
       Use:   "newcmd",
       Short: "...",
       RunE: func(cmd *cobra.Command, args []string) error {
           // 命令逻辑
       },
   }
   ```

2. 在 `cmd/root.go` 的 `init()` 中注册
   ```go
   rootCmd.AddCommand(newCmd)
   ```

3. 添加相应的测试和文档

### 7.3 性能优化建议

| 优化项 | 难度 | 收益 | 备注 |
|--------|------|------|------|
| 对象池 | ★★ | 5-10% | 减少 GC 压力 |
| 多数据库连接 | ★★★ | 10-20% | SQLite/H2 并发写入 |
| SIMD 加速 | ★★★★ | 5-15% | 数据转换加速 |
| 分布式处理 | ★★★★★ | 线性 | 跨机器处理 |

---

## 8. 故障处理

### 8.1 常见错误和恢复

| 错误 | 原因 | 恢复策略 |
|------|------|---------|
| OOM | 缓冲区过大 | 减少 buffer size 或 worker 数 |
| Deadlock | 多表并发 | 确保哈希路由正确 |
| Slow query | DB 连接慢 | 增加 `--slow-threshold` |
| 丢事件 | 过滤规则错误 | 检查 `--schema-table-regex` |

### 8.2 日志和调试

**监控输出**：
```bash
# 慢方法日志
[SLOW] processBinlog took 2.5s, args: ...

# 大事件日志
[WARN] Large event detected: log-pos=1234567, size=10MB

# 进度日志
[进度] 耗时: 1m30s | 进度: 45.2% | 速率: 938.9 events/sec | ETA: 1m40s

# 完成统计
[性能指标]
  总耗时: 1m40s
  处理事件: 188049 个
  导出事件: 188049 个
  平均速率: 1877.40 events/sec
  导出成功率: 100.00%
```

---

## 9. 测试策略

### 9.1 单元测试覆盖

| 模块 | 测试用例 | 目的 |
|------|---------|------|
| cache | 缓存命中、缓存失效、LRU 淘汰 | 正确性 |
| filter | 范围匹配、worker 路由 | 正确性 |
| util | SQL 生成、数据转换、范围解析 | 边界情况 |
| processor | 并发处理、背压、错误处理 | 并发安全 |

### 9.2 集成测试

```bash
# 完整流程测试
make test

# 性能基准测试
binlogx export --source large.bin --type sqlite --batch-size 2000
```

---

## 10. 部署建议

### 10.1 硬件配置

```
低端服务器（2-4 核）:
├─ --workers 4
├─ --batch-size 500
└─ 内存占用: ~100MB

中端服务器（8-16 核）:
├─ --workers 8-16
├─ --batch-size 2000
└─ 内存占用: ~500MB

高端服务器（32+ 核）:
├─ --workers 32
├─ --batch-size 5000
└─ 内存占用: ~1GB+
```

### 10.2 网络配置

```
离线模式: 无需网络，本地文件读取
在线模式: 需要到 MySQL 的网络连接
├─ TCP 连接: root@tcp(host:3306)
├─ 端口: 3306（默认）
└─ 权限: SELECT, SHOW TABLES, SHOW COLUMNS
```

---

## 11. 总结

### 核心设计原则

| 原则 | 实现 | 收益 |
|------|------|------|
| 解耦 | 接口驱动、模块化 | 易扩展 |
| 性能 | 并发、缓存、优化 | 高吞吐 |
| 稳定 | 测试、监控、恢复 | 可靠 |
| 易用 | CLI 友好、文档完整 | 低学习成本 |

### 技术栈

```
语言: Go 1.25+
框架: Cobra (CLI)
数据库驱动: github.com/go-sql-driver/mysql
其他: sync, context, regexp 等标准库
```

### 关键指标

| 指标 | 值 | 备注 |
|------|-----|------|
| 吞吐量 | 1500-2000 events/sec | 取决于硬件 |
| 内存占用 | 100MB-1GB | 可配置 |
| 延迟 | <100ms | 单次事件处理 |
| 可靠性 | 100% | 无丢事件（验证） |

---

**文档版本**: 1.0
**最后更新**: 2025-10-31
**维护者**: Architecture Team
