package benchmark

// DefaultTasks 默认 benchmark 任务集。
//
// 设计原则：
//   - 覆盖 5 类任务（问答/修改/重构/调试/多文件）
//   - 每类 3 个任务，难度分布 easy/medium/hard
//   - 预期关键词明确，可自动判断成功
//   - 不依赖特定代码库（通用任务）
//
// 真实使用时，可在此基础上扩展项目专属任务（指向具体文件）。
var DefaultTasks = []Task{
	// ===== 问答类 =====
	{
		ID:          "q-easy-1",
		Type:        TaskQuestion,
		Difficulty:  DiffEasy,
		Description: "解释一个简单函数的作用",
		Input:       "请解释 fmt.Println 和 fmt.Printf 的区别",
		ExpectedKeywords: []string{"Println", "Printf", "格式"},
		Timeout:     15,
	},
	{
		ID:          "q-medium-1",
		Type:        TaskQuestion,
		Difficulty:  DiffMedium,
		Description: "解释一个设计模式",
		Input:       "什么是 Wilson 置信区间？为什么 A/B 测试要用它？",
		ExpectedKeywords: []string{"Wilson", "置信", "区间", "样本"},
		Timeout:     20,
	},
	{
		ID:          "q-hard-1",
		Type:        TaskQuestion,
		Difficulty:  DiffHard,
		Description: "架构权衡分析",
		Input:       "对比 epsilon-greedy 和 UCB 在多臂老虎机中的优劣，何时该用哪个？",
		ExpectedKeywords: []string{"epsilon", "UCB", "探索", "利用"},
		Timeout:     30,
	},

	// ===== 修改类 =====
	{
		ID:          "e-easy-1",
		Type:        TaskEdit,
		Difficulty:  DiffEasy,
		Description: "修改变量名",
		Input:       "把 utils.go 里的变量 tmp 改成 buffer",
		ExpectedKeywords: []string{"buffer"},
		ExpectedFiles:   []string{"utils.go"},
		Timeout:         20,
	},
	{
		ID:          "e-medium-1",
		Type:        TaskEdit,
		Difficulty:  DiffMedium,
		Description: "添加错误处理",
		Input:       "给 readFile 函数加上文件不存在的错误处理，返回空切片而非 panic",
		ExpectedKeywords: []string{"error", "return"},
		Timeout:     25,
	},
	{
		ID:          "e-hard-1",
		Type:        TaskEdit,
		Difficulty:  DiffHard,
		Description: "并发安全改造",
		Input:       "把全局 cache map 改成 sync.Map 或加 mutex，保证并发安全",
		ExpectedKeywords: []string{"sync", "Mutex", "Map"},
		Timeout:     40,
	},

	// ===== 重构类 =====
	{
		ID:          "r-easy-1",
		Type:        TaskRefactor,
		Difficulty:  DiffEasy,
		Description: "提取常量",
		Input:       "把代码里硬编码的 100 提取成常量 MaxRetries",
		ExpectedKeywords: []string{"MaxRetries", "const"},
		Timeout:     20,
	},
	{
		ID:          "r-medium-1",
		Type:        TaskRefactor,
		Difficulty:  DiffMedium,
		Description: "提取函数",
		Input:       "把 process 函数里 50 行的校验逻辑提取成 validateInput 函数",
		ExpectedKeywords: []string{"func", "validate"},
		Timeout:     30,
	},
	{
		ID:          "r-hard-1",
		Type:        TaskRefactor,
		Difficulty:  DiffHard,
		Description: "接口抽象",
		Input:       "把直接依赖 MySQL 的代码抽象成 Storage 接口，方便测试时 mock",
		ExpectedKeywords: []string{"interface", "Storage"},
		Timeout:     45,
	},

	// ===== 调试类 =====
	{
		ID:          "d-easy-1",
		Type:        TaskDebug,
		Difficulty:  DiffEasy,
		Description: "nil 指针",
		Input:       "这段代码 panic: nil pointer，帮我找原因：return user.Name（user 可能为 nil）",
		ExpectedKeywords: []string{"nil", "检查", "if"},
		Timeout:     20,
	},
	{
		ID:          "d-medium-1",
		Type:        TaskDebug,
		Difficulty:  DiffMedium,
		Description: "goroutine 泄漏",
		Input:       "服务运行 2 小时后内存爆掉，怀疑 goroutine 泄漏，帮我排查这段带 channel 的代码",
		ExpectedKeywords: []string{"goroutine", "channel", "泄漏", "close"},
		Timeout:     30,
	},
	{
		ID:          "d-hard-1",
		Type:        TaskDebug,
		Difficulty:  DiffHard,
		Description: "并发竞态",
		Input:       "偶发数据不一致，pprof 显示 race condition，帮我分析这段并发写 map 的代码",
		ExpectedKeywords: []string{"race", "Mutex", "并发"},
		Timeout:     40,
	},

	// ===== 多文件类 =====
	{
		ID:          "m-easy-1",
		Type:        TaskMultiFile,
		Difficulty:  DiffEasy,
		Description: "重命名跨文件",
		Input:       "把 UserRepo 重命名成 UserRepository，涉及 user_repo.go 和 service.go 两个文件",
		ExpectedKeywords: []string{"UserRepository"},
		ExpectedFiles:   []string{"user_repo.go", "service.go"},
		Timeout:         30,
	},
	{
		ID:          "m-medium-1",
		Type:        TaskMultiFile,
		Difficulty:  DiffMedium,
		Description: "新增模块",
		Input:       "新增一个 audit 模块：audit.go（接口）+ audit_file.go（文件实现）+ audit_test.go（测试）",
		ExpectedKeywords: []string{"audit", "interface"},
		ExpectedFiles:   []string{"audit.go", "audit_file.go"},
		Timeout:         45,
	},
	{
		ID:          "m-hard-1",
		Type:        TaskMultiFile,
		Difficulty:  DiffHard,
		Description: "跨模块重构",
		Input:       "把 auth 模块从 session-based 改成 JWT，涉及 auth.go, middleware.go, handler.go, config.yaml",
		ExpectedKeywords: []string{"JWT", "token"},
		ExpectedFiles:   []string{"auth.go", "middleware.go"},
		Timeout:         60,
	},
}

// NewDefaultSuite 用默认任务集创建套件
func NewDefaultSuite(runner Runner) *Suite {
	s := NewSuite(runner)
	for _, t := range DefaultTasks {
		s.AddTask(t)
	}
	return s
}
