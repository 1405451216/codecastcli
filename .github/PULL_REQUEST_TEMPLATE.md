## 变更概述

简明描述本 PR 做了什么。

## 关联 Issue

Closes #（Issue 编号，如无请删除此行）

## 变更类型

- [ ] 新功能（feat）
- [ ] Bug 修复（fix）
- [ ] 重构（refactor，不改变行为）
- [ ] 文档（docs）
- [ ] 测试（test）
- [ ] 构建 / CI（chore）

## 变更详情

详细描述本 PR 的变更内容，包括：
- 新增/修改了哪些文件
- 关键设计决策

## 验证步骤

描述如何验证本 PR 的正确性：

```bash
# 例如：
go build ./...
go test ./internal/mypackage/ -v -count=1
```

## Checklist

- [ ] `go build ./...` 零错误
- [ ] `go test ./... -count=1` 全部通过
- [ ] `go vet ./...` 无警告
- [ ] 新增公开 API 已写文档注释
- [ ] 新增测试已覆盖关键路径
- [ ] CHANGELOG.md 已更新（如适用）

## 截图（如适用）

如有 UI 变更，请附上截图。
