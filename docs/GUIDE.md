# 文档导航指南

binlogx 项目文档已按以下方式组织，帮助你快速找到所需信息。

## 📚 文档地图

### 1. 快速开始（5 分钟）
👉 **[README.md](README.md)** - 项目首页
- 项目特性概览
- 安装和构建方式
- 基础使用示例（各命令演示）
- 全局参数说明
- 分库表范围匹配用法

### 2. 需求和规格（10 分钟）
👉 **[需求.MD](需求.MD)** - 项目需求文档
- 项目定位和功能总览
- 所有命令的参数清单
- 分库表范围匹配详解
- 核心机制说明
- 交付物清单

### 3. 命令行参考（快速查询）
👉 **[docs/CLI_REFERENCE.md](docs/CLI_REFERENCE.md)** - 完整命令手册
- 全局选项详解
- 各命令的详细说明（stat/parse/sql/export/rollback-sql/version）
- 导出格式详解（CSV/SQLite/H2/Hive/ES）
- 使用场景示例
- 常见错误和解决方案

### 4. 架构设计（深入理解）
👉 **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** - 系统架构文档
- 整体架构设计和数据流
- 8 个核心模块详解
- 并发处理机制
- 性能优化策略
- 设计决策和权衡
- 扩展性设计
- 部署建议

---

## 🎯 按用途查找

### "我想快速上手"
1. 阅读 [README.md](README.md) 安装部分
2. 查看 [README.md](README.md) 使用示例部分
3. 运行 `./bin/binlogx stat --source file.bin`

### "我想了解某个命令的用法"
1. 去 [docs/CLI_REFERENCE.md](docs/CLI_REFERENCE.md) 查对应命令
2. 查看参数说明和使用示例
3. 参考"使用场景"部分的实际例子

### "我想优化导出性能"
1. 阅读 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) 中的"性能优化"章节
2. 查看 [docs/CLI_REFERENCE.md](docs/CLI_REFERENCE.md) 中的"性能优化建议"
3. 根据硬件配置调整 `--batch-size` 和 `--workers`

### "我想扩展功能（添加导出格式）"
1. 阅读 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) 中的"业务处理器"章节
2. 查看"扩展性设计"部分的添加新导出格式步骤
3. 参考 `cmd/export_impl.go` 中现有的导出器实现

### "我想理解系统设计"
→ 完整阅读 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

---

## 📋 文档对应关系

| 您的问题 | 推荐文档 | 章节 |
|---------|---------|------|
| 怎样安装？ | README.md | 安装 |
| 怎样使用 stat 命令？ | CLI_REFERENCE.md | stat 命令 |
| 什么是分库表范围匹配？ | 需求.MD | 四、分库表范围匹配 |
| 导出性能如何优化？ | ARCHITECTURE.md | 5. 性能优化 |
| 系统如何处理并发？ | ARCHITECTURE.md | 4.2 并发处理详解 |
| 如何添加新的导出格式？ | ARCHITECTURE.md | 7.1 添加新的导出格式 |
| 支持哪些导出格式？ | CLI_REFERENCE.md | export 命令 |
| 分片锁有什么作用？ | ARCHITECTURE.md | 3.8 工具组件 |
| 缓存如何工作？ | ARCHITECTURE.md | 3.5 缓存层 |

---

## 🔍 按技术方面查找

### 并发和性能
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 3.3 事件处理管道
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 4.2 并发处理详解
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 5. 性能优化

### 数据源和输入
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 3.1 数据源层
- [README.md](README.md) - 全局参数

### 导出和输出
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 3.7 业务处理器
- [CLI_REFERENCE.md](docs/CLI_REFERENCE.md) - export 命令
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 4.1 导出流程

### 缓存和优化
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 3.5 缓存层
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 5.1 缓存优化

### 过滤和路由
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 3.4 过滤层
- [需求.MD](需求.MD) - 四、分库表范围匹配
- [README.md](README.md) - 分库表范围匹配

### 监控和调试
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 3.6 监控层
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 8. 故障处理

---

## 💡 学习路径

### 初学者
1. 阅读 [README.md](README.md) 了解概况 (5 min)
2. 按照示例使用各个命令 (10 min)
3. 查阅 [docs/CLI_REFERENCE.md](docs/CLI_REFERENCE.md) 了解详细参数 (15 min)

### 中级用户
1. 阅读 [ARCHITECTURE.md](docs/ARCHITECTURE.md) 理解设计 (30 min)
2. 查看 [ARCHITECTURE.md](docs/ARCHITECTURE.md) 性能优化部分 (10 min)
3. 根据硬件配置调整参数 (5 min)

### 高级开发者
1. 深入阅读 [ARCHITECTURE.md](docs/ARCHITECTURE.md) 全文 (60 min)
2. 阅读源代码中的关键模块
3. 参考扩展性设计部分扩展功能 (根据需求)

---

## 📖 文档大小参考

| 文档 | 大小 | 阅读时间 | 适合对象 |
|------|------|---------|---------|
| README.md | 8.5 KB | 5-10 min | 所有人 |
| 需求.MD | 5.9 KB | 5-10 min | 所有人 |
| CLI_REFERENCE.md | 8.8 KB | 10-15 min | 用户 |
| ARCHITECTURE.md | 22 KB | 30-60 min | 开发者 |

---

## ❓ 常见问题速查

**Q: 怎样导出为 SQLite？**
→ [CLI_REFERENCE.md](docs/CLI_REFERENCE.md) - export 命令 - SQLite 示例

**Q: 什么是 batch-size？**
→ [需求.MD](需求.MD) - 5.1 export - batch-size 参数

**Q: 如何过滤特定的库表？**
→ [README.md](README.md) - 分库表范围匹配

**Q: 为什么导出速度慢？**
→ [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 5. 性能优化

**Q: 支持多少个 worker？**
→ [README.md](README.md) - 全局参数 - workers

**Q: 如何计算进度 ETA？**
→ [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 4.1 导出流程

**Q: 系统的内存占用多少？**
→ [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 10.1 硬件配置

**Q: 如何添加新命令？**
→ [ARCHITECTURE.md](docs/ARCHITECTURE.md) - 7.2 添加新的命令

---

## 🔗 快速链接

| 类型 | 链接 |
|------|------|
| 项目主页 | [README.md](README.md) |
| 需求规格 | [需求.MD](需求.MD) |
| 命令参考 | [docs/CLI_REFERENCE.md](docs/CLI_REFERENCE.md) |
| 架构设计 | [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) |
| GitHub | https://github.com/aitoooooo/binlogx |

---

**最后更新**：2025-10-31
**文档版本**：1.0
