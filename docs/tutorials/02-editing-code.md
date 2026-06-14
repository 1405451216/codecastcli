# 编辑代码

## 基本编辑

AI 通过 `edit_file` 工具修改代码：

```
> 把 UserService 的 GetAll 方法改为支持分页
```

AI 会：
1. 通过索引找到相关文件
2. 读取文件内容
3. 生成 diff 并应用

## 多文件编辑

```
> 重构：把 database.go 中的连接池逻辑抽到 db/pool.go
```

AI 会自动处理多个文件的创建、修改和删除。

### @file 精确控制

指定 AI 只能看到和修改的文件：

```
> 看看 @src/auth/handler.go 和 @src/auth/middleware.go 有没有重复逻辑，合并一下
```

## 代码审查

让 AI 审查变更：

```
> /review main        # 审查当前分支相对 main 的所有变更
> /review             # 审查未提交的变更
```

输出示例：
```
## 代码审查报告

### src/auth/handler.go (3 issues)

⚠️ [warning] 第 45 行：未对 token 做过期检查
   建议：添加 token.ExpiresAt < time.Now() 判断

❌ [error] 第 78 行：SQL 注入风险
   建议：使用参数化查询替代字符串拼接
```

## Git 集成

### blame 分析

```
> /blame src/auth/handler.go
```

将 Git blame 历史注入上下文，AI 可以理解每行代码的修改者和原因。

### commit 历史

```
> /history src/auth/
```

将目录的 commit 历史注入上下文，帮助 AI 理解演化过程。

### 分支 diff

```
> /diff feature/login
```

分析当前分支与目标分支的差异。

## 撤销与回滚

### 自动 Checkpoint

每次 AI 修改文件前，自动创建 checkpoint：

```
> /undo              # 撤销上一步修改
> /undo list         # 查看所有可撤销点
> /undo to <id>      # 回滚到指定 checkpoint
```

### Git 自动提交

Codecast 在每次有意义的修改后自动 `git add + commit`（可配置）。

## 安全模式

### Safe Mode

限制 AI 的操作范围：

```bash
codecast --safe
```

在 safe 模式下：
- AI 不能执行 shell 命令
- 不能修改 `.env`、`config` 等敏感文件
- 每个操作都需要确认

### Scope 限制

```bash
codecast --scope ./src
```

AI 只能读取和修改 `./src` 目录下的文件。

## 下一步

- [工具全解](03-using-tools.md)
- [双 Agent 协作](04-multi-agent.md)
