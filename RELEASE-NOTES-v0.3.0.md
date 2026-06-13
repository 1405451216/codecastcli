# Release v0.3.0

**发布日期**: 2026-06-13
**代号**: PromptForge（提示词工坊）
**重点**: 提示词 A/B 框架 + 11 个内置变体 + /prompt 斜杠命令

---

## 🌟 核心特性

### 提示词 A/B 框架

Codecast 引入完整的**提示词 A/B 测试与外部化框架**——可在不重编译的情况下热替换系统提示词，并按变体维度统计成本/调用量。

**11 个内置变体**（按场景分类）：

| 类别 | 变体 | 适用场景 |
|------|------|----------|
| 通用 | `default` | 平衡（推荐） |
| 通用 | `concise` | 极简，省 token |
| 通用 | `safety-first` | 保守 + 强制验证 |
| Claude | `claude-style` | XML 章节 + good/bad 对照 |
| Claude | `decision-tree` | 5 步评估用户意图 |
| Claude | `self-check` | 5 步自检防低级错误 |
| Claude | `mcp-router` | MCP vs 内置工具路由 |
| Claude | `mentor-coach` | 温暖 + 建设性 push back |
| 专精 | `code-reviewer` | 分级反馈（🔴/🟡/🟢） |
| 专精 | `pair-programmer` | 教学式注解 |
| 安全 | `scope-guard` | 文件访问 scope 强制检查 |

### 4 种外部化方式（按优先级）

1. 编译时嵌入（fallback）
2. `~/.codecast/prompts/*.yaml`（用户级）
3. 项目级 `.codecast/prompts/*.yaml`（自动按 cwd 加载）
4. `--prompt-project-dir` 覆盖

### 3 种选择策略

- `fixed`：按名指定
- `round-robin`：按权重轮转
- `weighted`：按权重概率选

---

## 🚀 快速使用

```bash
# 切到极简模式
/prompt use concise

# 加权 A/B 测试（90% default + 10% concise）
/prompt use default
/prompt use concise
codecast --prompt-strategy weighted --prompt-weight default=9 --prompt-weight concise=1

# 预览变体效果
/prompt show safety-first

# 不退出 REPL 重新加载
/prompt reload

# 看 A/B 数据
/cost by-variant
```

**CLI 快捷方式**：
```bash
codecast --prompt-variant concise
CODECAST_PROMPT_VARIANT=safety-first codecast
```

**配置文件**（`~/.codecast/config.yaml`）：
```yaml
prompt_variant: claude-style
prompt_strategy: weighted
prompt_weights:
  default: 5
  concise: 1
```

---

## 📊 统计

- 35 个 Go 包
- 60+ 新测试用例
- 5 个 CI 目标（linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64）
- 5 次 commit（从 v0.2.0 到 v0.3.0）
- 11 个内置变体

---

## 🐛 修复

- F-01 至 F-10 + F-12（v0.1.0 已记录，本版本 14 项修复）
- 4 个新发现的 bug：round-robin 永远返回第一项 / EmbeddedVariants 不调 Parse / COALESCE 不替换空串 / RecentRecords 没读 prompt_variant
- 1 个发布前发现的真实 bug：`wizard/readinput_unix.go` 用了被移除的 `syscall.ReadPassword`，改用 `golang.org/x/term`

---

## 📥 下载

- **Linux x86_64**: `codecast-linux-amd64` (21 MB)
- **Linux ARM64**: `codecast-linux-arm64` (20 MB)
- **macOS Intel**: `codecast-darwin-amd64` (22 MB)
- **macOS Apple Silicon**: `codecast-darwin-arm64` (21 MB)
- **Windows x86_64**: `codecast-windows-amd64.exe` (22 MB)

```bash
# macOS / Linux
tar -xzf codecast-0.3.0-<platform>.tar.gz
sudo mv codecast /usr/local/bin/

# Windows：把 codecast-windows-amd64.exe 改名 codecast.exe 加到 PATH
```

---

## 🙏 致谢

v0.3.0 大量借鉴了 Anthropic Claude Fable 5 系统提示词的设计模式
（XML 章节化、good/bad 对照、决策流、自检清单、人格层）。
**保留的模式 → 变体**：
- XML 章节 → `claude-style`
- good/bad 对照 → `claude-style` 工具段
- 决策树评估 → `decision-tree`
- 回复前自检 → `self-check`
- MCP 工具路由 → `mcp-router`
- 分级反馈 → `code-reviewer`
- 对话式注解 → `pair-programmer`
- 安全边界 → `scope-guard`
- 人格层（温暖 + push back + 边界感）→ `mentor-coach`

---

## ⬆️ 升级指引

从 v0.2.0 升级无破坏性变更：
- 现有配置文件兼容
- 现有 `~/.codecast/prompts/` 目录（如有）会自动加载
- 不指定 variant 时 fallback 到 `default`
- 不指定 strategy 时 fallback 到 `fixed`

直接覆盖安装二进制即可。

---

**完整变更日志**: 见 [CHANGELOG.md](CHANGELOG.md)
**使用文档**: 见 [README.md](README.md) 的"提示词 A/B 框架"章节
