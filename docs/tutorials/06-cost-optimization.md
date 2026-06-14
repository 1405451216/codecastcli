# 成本优化

## 预算控制

### 会话级预算

```bash
codecast --budget 5.0    # 本次会话最多花费 $5
```

超过预算后，AI 拒绝新请求但允许 `/cost`、`/session` 等查询命令。

### 日级预算

```yaml
# ~/.codecast/config.yaml
budget_daily_limit: 20.0    # 每天最多 $20
budget_session_limit: 5.0   # 每次会话最多 $5
```

### 查看费用

```
> /cost summary        # 总览
> /cost daily          # 按日统计
> /cost list           # 详细记录
> /cost by-variant     # 按提示词变体统计
```

## 智能模型路由

### 自动路由

```
> /route               # 启用智能路由
```

开启后，Codecast 根据任务复杂度自动选择模型：

| 任务类型 | 示例 | 推荐模型 | 预估成本 |
|---------|------|---------|---------|
| 简单问答 | "Go 的 for 循环语法" | gpt-4o-mini | $0.001 |
| 代码编辑 | "添加一个函数" | gpt-4o | $0.01 |
| 复杂重构 | "重构整个 auth 模块" | claude-3.5-sonnet | $0.05 |
| 架构设计 | "设计微服务架构" | claude-3-opus | $0.10 |

### 路由配置

```yaml
# ~/.codecast/config.yaml
routing:
  enabled: true
  rules:
    - pattern: "简单|解释|什么是"
      model: gpt-4o-mini
    - pattern: "重构|架构|设计"
      model: claude-3.5-sonnet
  cache_ttl: 300    # 路由缓存 5 分钟
```

### 节省效果

根据实际统计，智能路由平均节省 **40-60%** 成本。

## 上下文压缩

### 自动摘要

当对话历史超过 Token 限制时，Codecast 自动摘要压缩：

```
> /config get context_max_tokens    # 查看当前限制
> /config set context_max_tokens 8000
```

压缩策略：
1. 保留最近 5 轮完整对话
2. 早期对话压缩为摘要
3. 工具调用结果保留关键输出

### 预算感知压缩

开启后，按剩余预算动态调整上下文长度：

```yaml
# ~/.codecast/config.yaml
budget_aware_compression: true
```

- 预算充足：保留完整上下文
- 预算紧张：积极压缩，减少 Token 消耗

## 省 Token 技巧

1. **使用 concise 变体** — AI 回答更简短
   ```
   > /prompt use concise
   ```

2. **@file 精确引用** — 只注入需要的文件
   ```
   > 看看 @src/auth/handler.go 第 50-80 行
   ```

3. **scope 限制** — 缩小索引范围
   ```bash
   codecast --scope ./src
   ```

4. **及时清理** — 删除不需要的会话记录
   ```
   > /cost clear
   ```

5. **小模型优先** — 简单任务不需要强模型
   ```
   > /model gpt-4o-mini
   ```

## 成本监控

### CI 集成

在 CI 中设置预算告警：

```yaml
# .github/workflows/codecast-budget.yml
- name: Check budget
  run: |
    codecast exec "检查项目状态" --budget 1.0
    if [ $? -ne 0 ]; then
      echo "::warning::Budget exceeded"
    fi
```

### 费用报告

```
> /cost summary
```

```
📊 成本报告
━━━━━━━━━━━━━━━━━━━━━
今日: $3.20 / $20.00 (16%)
本次会话: $0.85 / $5.00 (17%)
━━━━━━━━━━━━━━━━━━━━━
请求数: 42
平均每次: $0.020
最贵模型: claude-3.5-sonnet ($0.45)
最省模型: gpt-4o-mini ($0.08)
```
