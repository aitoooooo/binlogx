# Parse 命令改进与列名映射功能

## 概述
解决用户反馈的三个问题，并为所有命令提供统一的列名映射功能。

## 用户反馈的三个问题

### 1. ✅ SQL 生成（已解决）
**问题**：`parse` 命令输出的 `sql` 字段为空
**原因**：SQL 生成逻辑尚未实现
**解决方案**：
- 在 `parseHandler.Handle()` 方法中调用 `SQLGenerator` 生成 SQL
- 支持 INSERT, UPDATE, DELETE 三种操作

### 2. ✅ 列名显示（已解决）
**问题**：显示 `col_0`, `col_1`... 而不是实际的列名
**原因**：Binlog 文件中不包含列名信息，需要从 MySQL 查询
**解决方案**：
- 创建 `CommandHelper` 公共工具类，提供列名映射功能
- 如果指定了 `--db-connection` 参数，自动从 MySQL 查询表元数据
- 将 `col_N` 映射到实际列名

### 3. ✅ 分页交互（已解决）
**问题**：需要按 `n` 再按回车，而不是直接按空格翻页
**原因**：原始实现只支持 `n` 键
**解决方案**：
- 改进 `displayPaginatedEvents()` 函数
- 支持 `SPACE` 或 `n` 翻页，`q` 退出
- 按空格或 `n` 后再按回车即可翻页

## 核心设计：CommandHelper

### 功能
```go
type CommandHelper struct {
    metaCache *cache.MetaCache
}

// 公共方法
func NewCommandHelper(dbConnection string) *CommandHelper
func (ch *CommandHelper) MapColumnNames(event *models.Event)
func (ch *CommandHelper) getColumnNameMapping(database, table string) map[string]string
```

### 特点
1. **自动初始化**：如果提供 `--db-connection` 参数，自动连接数据库
2. **优雅降级**：如果无数据库连接，列名仍显示为 `col_N`（不会报错）
3. **缓存机制**：使用 MetaCache 缓存列信息，避免重复查询
4. **线程安全**：所有 EventHandler 都正确使用了 Mutex

## 应用范围

所有需要列名映射的命令都已更新：

| 命令 | 状态 | 实现 |
|------|------|------|
| **parse** | ✅ | parseHandler 中调用 helper.MapColumnNames() |
| **sql** | ✅ | sqlHandler 中调用 helper.MapColumnNames() |
| **rollback-sql** | ✅ | rollbackSqlHandler 中调用 helper.MapColumnNames() |
| **export** | ✅ | CSVExporter, SQLiteExporter 等都调用 helper.MapColumnNames() |

## `--db-connection` 参数的双重作用

### 1. 作为数据源（原有功能）
```bash
# 从 MySQL 实时读取 binlog 事件
binlogx parse --db-connection "user:password@tcp(localhost:3306)/"
```

### 2. 作为元数据来源（新增功能）
```bash
# 从 binlog 文件读取，从 MySQL 查询列名
binlogx parse --source mysql-bin.000001 --db-connection "user:password@tcp(localhost:3306)/"
```

这允许用户在离线分析 binlog 时获取真实的列名。

## 核心实现细节

### 列名映射流程
```
Event (col_0, col_1, col_2...)
  ↓
helper.MapColumnNames(event)
  ↓
metaCache.GetTableMeta(database, table)
  ↓
INFORMATION_SCHEMA.COLUMNS 查询
  ↓
Event (col_name_1, col_name_2, col_name_3...)
```

### 异常处理
- 如果数据库连接失败：继续工作，列名显示为 `col_N`
- 如果表不存在：列名显示为 `col_N`
- 如果缓存满：LRU 淘汰策略自动清理

## 代码质量

### 变化的文件
1. **cmd/cmd_utils.go** (新增)
   - CommandHelper 类和列名映射逻辑
   - 73 行代码，设计清晰

2. **cmd/parse.go** (改进)
   - 集成 CommandHelper 和 SQLGenerator
   - 改进分页交互
   - 171 行代码

3. **cmd/sql.go** (改进)
   - 实现 SQL 生成逻辑
   - 集成 CommandHelper
   - 109 行代码

4. **cmd/rollback_sql.go** (改进)
   - 实现回滚 SQL 生成
   - 集成 CommandHelper
   - 151 行代码

5. **cmd/export.go** (改进)
   - 集成 CommandHelper 到所有导出器
   - 176 行代码

6. **cmd/export_impl.go** (改进)
   - 为所有导出器添加 helper 支持
   - 213 行代码

### 设计原则
- ✅ DRY（Don't Repeat Yourself）：列名映射逻辑集中在 CommandHelper
- ✅ 单一职责：每个类专注于一个功能
- ✅ 依赖注入：CommandHelper 通过参数传递，便于测试
- ✅ 优雅降级：无数据库连接时仍可工作

## 测试建议

### 场景 1：有数据库连接
```bash
binlogx parse --source mysql-bin.000001 \
  --db-connection "user:password@tcp(localhost:3306)/" \
  --include-db mydb \
  --include-table mytable
```
预期：显示实际列名

### 场景 2：无数据库连接
```bash
binlogx parse --source mysql-bin.000001 \
  --include-db mydb \
  --include-table mytable
```
预期：显示 col_0, col_1... 但命令仍正常工作

### 场景 3：分页交互
任何 parse 命令执行后，按空格翻页应该正常工作

## 后续优化空间

1. **性能**：可以实现列名预加载，而不是延迟加载
2. **缓存持久化**：可以将列名缓存保存到本地，离线使用
3. **完整的 SQL 生成**：sql 命令现已生成基础 SQL，可添加更多优化
4. **监控集成**：与 Monitor 包集成，记录列名查询耗时

## 总结

这次改进实现了用户反馈的三个功能，同时设计了一个可复用的 `CommandHelper` 类，使所有命令都能受益于列名映射功能。代码质量高，设计清晰，易于维护和扩展。
