# 子项目 A：LLM 路由 — 设计文档

## 背景

CodecastCLI 已支持 7 个 LLM 提供商（openai、anthropic、ollama、deepseek、qwen、siliconflow、custom），但每次只能使用一个模型。子代理（code-reviewer、researcher、test-runner、bug-fixer）在 dispatch 时始终使用主 agent 的模型，无法针对任务特点匹配合适的模型。

**目标**：允许每个子代理使用独立的模型和提供商，支持用户通过配置文件覆盖。

## 设计概览

```
SubAgentConfig (代码内置)           .codecast/models.json (可选覆盖)
  code-reviewer:                     code-reviewer: { model: "claude-3-5-sonnet" }
    model: {                         (不写就用代码默认值)
      model: "claude-sonnet-4",
      provider: "anthropic"         ↓
    }                         SubAgentDispatcher.dispatch(agentName)
        ↓                                  ↓
              resolveEffectiveModel(agentName)
                  优先级：models.json > agent.model > 主 agent 模型
                        ↓
                LLMClient.fromConfig(modelRef)
                        ↓
                new AgentLoop(llmClient, ...)
```

**两个核心模块**：
1. `ModelRef` 值对象 + `LLMClient.fromConfig()` 工厂方法
2. `SubAgentConfig.model` 字段 + `resolveEffectiveModel()` 解析函数

## 新增类型

### ModelRef

```typescript
// src/api/llm.ts
export interface ModelRef {
  /** 模型名称 */
  model: string
  /** 提供商标识 */
  provider: string
}

export class LLMConfigError extends Error {
  constructor(
    message: string,
    public modelRef: ModelRef,
    /** 错误原因：missing_key 可回退，其他不可回退 */
    public reason: 'missing_key' | 'missing_url' | 'unknown' = 'unknown',
  ) {
    super(message)
    this.name = 'LLMConfigError'
  }
}
```

### SubAgentConfig 扩展

```typescript
// src/agent/subagent.ts
export interface SubAgentConfig {
  name: string
  description: string
  systemPrompt: string
  tools: string[]
  maxRounds: number
  model?: ModelRef   // 内置默认模型（可选）
}
```

### models.json 格式

```json
{
  "code-reviewer": {
    "model": "claude-3-5-sonnet-20241022",
    "provider": "anthropic"
  },
  "researcher": {
    "model": "gpt-4o-mini"
  }
}
```

- 文件路径：`.codecast/models.json`（项目根目录）
- `model` 必填；`provider` 不填则回退到主 agent 的提供商
- 只覆盖需要定制的子代理，其余使用代码默认值

## 核心逻辑

### 模型解析优先级

```typescript
// src/agent/subagent.ts
import { ModelRef } from '../api/llm.js'

/**
 * 解析子代理生效的模型引用。
 *
 * 优先级：models.json 用户覆盖 > SubAgentConfig.model（代码默认） > fallback（主 agent 模型）
 */
export function resolveEffectiveModel(
  agentName: string,
  agentConfig: SubAgentConfig | undefined,
  overrides: Record<string, ModelRef>,
  fallback: ModelRef,
): ModelRef {
  // 1. 用户覆盖（models.json）
  const override = overrides[agentName]
  if (override) {
    return {
      model: override.model,
      // provider 不填则回退到主 agent 的 provider
      provider: override.provider || fallback.provider,
    }
  }

  // 2. 代码默认值（SubAgentConfig.model）
  if (agentConfig?.model) {
    // agentConfig.model.provider 可能也不填，统一回退到主 agent 的 provider
    return {
      model: agentConfig.model.model,
      provider: agentConfig.model.provider || fallback.provider,
    }
  }

  // 3. 兜底：主 agent 模型
  return fallback
}
```

### LLMClient.fromConfig 工厂方法

```typescript
// src/api/llm.ts — LLMClient 类上新增静态方法
/**
 * 从 ModelRef 创建 LLMClient 实例。
 * @param ref 模型引用
 * @param env 环境变量映射（默认 process.env，测试时可注入）
 */
static fromConfig(ref: ModelRef, env: Record<string, string | undefined> = process.env): LLMClient {
  const providerConfig = resolveProviderConfig(ref.provider)

  // API Key 解析：环境变量 > 占位符（无 key 的提供商）
  const apiKey = (providerConfig.envKeyVar ? env[providerConfig.envKeyVar] : undefined)
    || (providerConfig.requiresApiKey ? '' : 'sk-placeholder')

  // 缺少 key
  if (providerConfig.requiresApiKey && !apiKey) {
    throw new LLMConfigError(
      `子代理需要提供商 "${providerConfig.displayName}" 但缺少 API Key` +
      `（请设置 ${providerConfig.envKeyVar} 环境变量）`,
      ref,
      'missing_key',
    )
  }

  // base URL 解析
  const baseURL = (providerConfig.envBaseVar ? env[providerConfig.envBaseVar] : undefined)
    || providerConfig.defaultBaseURL

  if (!baseURL) {
    throw new LLMConfigError(
      `提供商 "${providerConfig.displayName}" 未配置 base URL`,
      ref,
      'missing_url',
    )
  }

  return new LLMClient({
    provider: providerConfig.apiFormat,
    apiKey,
    model: ref.model,
    baseURL,
  })
}
```

### SubAgentDispatcher.dispatch 集成

```typescript
// src/agent/subagent.ts — dispatch() 方法中
async dispatch(agentName: string, task: string, context?: string): Promise<SubAgentResult> {
  const agent = this.agents.get(agentName)
  if (!agent) return { ... }

  // 解析子代理模型
  let llmClient: LLMClient
  try {
    const modelRef = resolveEffectiveModel(
      agentName,
      agent,
      this.modelConfig.overrides,
      this.modelConfig.fallback.ref,
    )
    llmClient = LLMClient.fromConfig(modelRef)
  } catch (e) {
    if (e instanceof LLMConfigError && e.reason === 'missing_key') {
      // 仅缺少 API Key 时优雅降级，回退到主 agent 模型
      console.warn(`⚠ 子代理 "${agentName}" 回退到主 Agent 模型: ${e.message}`)
      llmClient = this.modelConfig.fallback.client
    } else {
      throw e  // missing_url 等错误不放行
    }
  }

  // 创建 AgentLoop（使用子代理专属的 LLMClient）
  const tools = this.toolRegistry!.getFiltered(agent.tools || [])
  const loop = new AgentLoop(llmClient, this.toolExecutor!, {
    maxIterations: agent.maxRounds,
    maxTimeoutMs: 120000,
    systemPrompt: agent.systemPrompt,
    tools,
    permManager: this.permManager!,
    hookManager: this.hookManager!,
    compactor: new ContextCompactor({ maxTokens: 80000, keepRecentRounds: 4 }),
  })

  const result = await loop.run(userMessage, [])
  return { ... }
}
```

## models.json 加载

在 `src/index.ts` 中启动时加载：

```typescript
// src/index.ts — 新增方法
private loadModelOverrides(): Record<string, ModelRef> {
  const file = resolve(process.cwd(), '.codecast', 'models.json')
  if (!existsSync(file)) return {}

  try {
    const raw = JSON.parse(readFileSync(file, 'utf-8'))
    const overrides: Record<string, ModelRef> = {}
    for (const [name, config] of Object.entries(raw)) {
      if (config && typeof (config as any).model === 'string') {
        overrides[name] = {
          model: (config as any).model,
          provider: (config as any).provider || '',  // 不填留空，交给 resolveEffectiveModel 统一回退
        }
      }
    }
    return overrides
  } catch (e) {
    console.warn(`⚠ .codecast/models.json 解析失败: ${(e as Error).message}`)
    return {}
  }
}
```

在 constructor 中调用，注入到 SubAgentDispatcher：

```typescript
// src/index.ts — constructor 中
this.applyModelConfig()
```

`applyModelConfig()` 方法（constructor 和 `/reload` 共用）：

```typescript
private applyModelConfig(): void {
  const overrides = this.loadModelOverrides()
  this.subAgentDispatcher.setModelConfig({
    overrides,
    fallback: {
      ref: { model: this.llm.getModel(), provider: this.llm.getProvider() },
      client: this.llm,
    },
  })
}
```

## /reload 命令（新增）

运行时重新加载 `models.json`，无需重启 CLI：

```typescript
case '/reload': {
  this.applyModelConfig()
  ui.printInfo('✅ models.json 已重新加载')
  break
}
```

## /model 命令

在 `src/index.ts` 的 slash command 处理中新增 `/model`：

```typescript
case '/model': {
  const main = `主 Agent: ${this.llm.getModel()} (${this.llm.getProvider()})`
  const lines = [main, '']
  const fallback = { model: this.llm.getModel(), provider: this.llm.getProvider() }

  for (const [name, agent] of this.subAgentDispatcher.getAgents()) {
    const effective = resolveEffectiveModel(name, agent, this.modelConfig.overrides, fallback)
    const source = this.modelConfig.overrides[name]
      ? ' ← models.json'
      : agent.model
        ? ' ← 代码默认值'
        : ' ← 主 Agent 模型'
    lines.push(`  ${name}: ${effective.model} (${effective.provider})${source}`)
  }

  lines.push('', '💡 修改 models.json 后用 /reload 热重载')
  ui.printInfo(lines.join('\n'))
  break
}
```

## 错误处理

| 场景 | 行为 |
|---|---|
| models.json 不存在 | 正常，使用代码默认值 |
| models.json JSON 解析失败 | warn + 使用代码默认值 |
| models.json 中 provider 不写 | 统一回退到主 agent 的 provider |
| 子代理模型缺少 API Key（missing_key） | 回退到主 agent 模型 + warn |
| 子代理模型缺少 base URL（missing_url） | 抛出异常，不回退 |
| models.json 被修改后执行 `/reload` | 运行时热加载，无需重启 |
| models.json 被修改后执行 `/model` | 提示"修改后请用 /reload 热重载" |
| 子代理 dispatch 成功（新模型） | 正常执行 |

## SubAgentDispatcher 新增字段和方法

```typescript
// src/agent/subagent.ts

/** 模型配置（一次性注入） */
export interface SubAgentModelConfig {
  /** models.json 用户覆盖 */
  overrides: Record<string, ModelRef>
  /** 主 agent 的基准模型（兜底用） */
  fallback: {
    ref: ModelRef       // 模型名 + 提供商
    client: LLMClient   // 已有 LLMClient 实例（降级回退时直接复用）
  }
}

export class SubAgentDispatcher {
  // ... 现有字段 ...

  private modelConfig: SubAgentModelConfig = {
    overrides: {},
    fallback: {
      ref: { model: 'gpt-4o', provider: 'openai' },
      client: null as unknown as LLMClient,
    },
  }

  /** 一次性注入模型配置 */
  setModelConfig(config: SubAgentModelConfig): void {
    this.modelConfig = config
  }

  /** 获取已注册的子代理（供 /model 命令使用） */
  getAgents(): Map<string, SubAgentConfig> {
    return this.agents
  }
}
```

## 内置子代理默认模型

| 子代理 | 推荐模型 | 推荐提供商 | 理由 |
|---|---|---|---|
| code-reviewer | claude-sonnet-4-20250514 | anthropic | 代码审查需要强推理 |
| researcher | gpt-4o-mini | openai | 信息检索不需要最强模型 |
| test-runner | gpt-4o | openai | 测试编写中等复杂度 |
| bug-fixer | claude-sonnet-4-20250514 | anthropic | Bug 修复需要强推理 |

如果用户不提供 models.json，这些默认值生效。

## 文件清单

| 文件 | 操作 | 说明 |
|---|---|---|
| `src/api/llm.ts` | 修改 | 新增 `ModelRef`、`LLMConfigError`、`LLMClient.fromConfig()` |
| `src/agent/subagent.ts` | 修改 | `SubAgentConfig.model`、`resolveEffectiveModel()`、SubAgentDispatcher 新增字段和方法 |
| `src/index.ts` | 修改 | models.json 加载、`/model` 命令、注入 modelOverrides 到 SubAgentDispatcher |
| `.codecast/models.json` | 新增 | 示例文件（空 JSON 或注释说明） |
| `tests/model-routing.test.ts` | 新增 | 覆盖优先级解析、fromConfig 创建（含 env 注入）、错误分类回退、`/model` 和 `/reload` 命令 |

## 验收标准

1. **模型解析优先级**：models.json > SubAgentConfig.model > 主 agent 模型
2. **跨提供商路由**：code-reviewer 用 anthropic、researcher 用 openai 可同时工作
3. **优雅降级**：子代理缺少 API Key（missing_key）时回退到主 agent 模型并 warn，其他错误抛出
4. **`/model` 命令**：显示主 agent 和所有子代理的实际生效模型及来源，底部提示 `/reload`
5. **`/reload` 命令**：运行时热重载 models.json，无需重启 CLI
6. **向后兼容**：不写 models.json、不给 SubAgentConfig 加 model 字段时，行为与当前完全一致