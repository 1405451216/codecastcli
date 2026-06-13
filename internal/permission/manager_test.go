package permission

import "testing"

func TestNewManager_SuggestMode(t *testing.T) {
	m := NewManager(ModeSuggest)

	if m.Mode() != ModeSuggest {
		t.Errorf("Mode() = %v, want %v", m.Mode(), ModeSuggest)
	}

	// 建议模式下所有工具都需要确认，ShouldApprove 返回 true
	tools := []string{"read_file", "list_dir", "grep_search", "glob_search",
		"web_request", "web_fetch", "write_file", "edit_file", "shell_execute",
		"mcp_some_tool", "unknown_tool"}

	for _, tool := range tools {
		if !m.ShouldApprove(tool) {
			t.Errorf("SuggestMode: ShouldApprove(%q) = false, want true", tool)
		}
	}
}

func TestNewManager_AutoEditMode(t *testing.T) {
	m := NewManager(ModeAutoEdit)

	if m.Mode() != ModeAutoEdit {
		t.Errorf("Mode() = %v, want %v", m.Mode(), ModeAutoEdit)
	}

	// 只读和编辑类工具自动放行，ShouldApprove 返回 false
	allowedTools := []string{"read_file", "list_dir", "grep_search", "glob_search",
		"web_request", "web_fetch", "write_file", "edit_file"}

	for _, tool := range allowedTools {
		if m.ShouldApprove(tool) {
			t.Errorf("AutoEditMode: ShouldApprove(%q) = true, want false", tool)
		}
	}

	// 危险和 MCP 工具需要确认，ShouldApprove 返回 true
	needConfirmTools := []string{"shell_execute", "mcp_some_tool", "mcp_another"}

	for _, tool := range needConfirmTools {
		if !m.ShouldApprove(tool) {
			t.Errorf("AutoEditMode: ShouldApprove(%q) = false, want true", tool)
		}
	}
}

func TestNewManager_FullAutoMode(t *testing.T) {
	m := NewManager(ModeFullAuto)

	if m.Mode() != ModeFullAuto {
		t.Errorf("Mode() = %v, want %v", m.Mode(), ModeFullAuto)
	}

	// 全自动模式下所有已知工具自动放行，ShouldApprove 返回 false
	knownTools := []string{"read_file", "list_dir", "grep_search", "glob_search",
		"web_request", "web_fetch", "write_file", "edit_file", "shell_execute"}

	for _, tool := range knownTools {
		if m.ShouldApprove(tool) {
			t.Errorf("FullAutoMode: ShouldApprove(%q) = true, want false", tool)
		}
	}

	// 未知工具在 full-auto 模式下也不需要确认
	if m.ShouldApprove("unknown_tool") {
		t.Errorf("FullAutoMode: ShouldApprove(%q) = true, want false", "unknown_tool")
	}
}

func TestParseApprovalMode(t *testing.T) {
	tests := []struct {
		input   string
		want    ApprovalMode
		wantErr bool
	}{
		{"suggest", ModeSuggest, false},
		{"SUGGEST", ModeSuggest, false},
		{"Suggest", ModeSuggest, false},
		{"auto-edit", ModeAutoEdit, false},
		{"autoedit", ModeAutoEdit, false},
		{"auto_edit", ModeAutoEdit, false},
		{"Auto-Edit", ModeAutoEdit, false},
		{"AUTOEDIT", ModeAutoEdit, false},
		{"full-auto", ModeFullAuto, false},
		{"fullauto", ModeFullAuto, false},
		{"full_auto", ModeFullAuto, false},
		{"Full-Auto", ModeFullAuto, false},
		{"FULLAUTO", ModeFullAuto, false},
		{"invalid", ModeSuggest, true},
		{"", ModeSuggest, true},
		{"auto", ModeSuggest, true},
	}

	for _, tt := range tests {
		got, err := ParseApprovalMode(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseApprovalMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseApprovalMode(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestNewManagerFromString(t *testing.T) {
	// 有效字符串
	validTests := []struct {
		input string
		want  ApprovalMode
	}{
		{"suggest", ModeSuggest},
		{"auto-edit", ModeAutoEdit},
		{"full-auto", ModeFullAuto},
	}

	for _, tt := range validTests {
		m, err := NewManagerFromString(tt.input)
		if err != nil {
			t.Errorf("NewManagerFromString(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if m.Mode() != tt.want {
			t.Errorf("NewManagerFromString(%q).Mode() = %v, want %v", tt.input, m.Mode(), tt.want)
		}
	}

	// 无效字符串
	_, err := NewManagerFromString("invalid-mode")
	if err == nil {
		t.Errorf("NewManagerFromString(%q) expected error, got nil", "invalid-mode")
	}
}

func TestManager_AddAutoAllow(t *testing.T) {
	m := NewManager(ModeSuggest)

	// 建议模式下 shell_execute 需要确认
	if !m.ShouldApprove("shell_execute") {
		t.Errorf("before AddAutoAllow: ShouldApprove(shell_execute) = false, want true")
	}

	m.AddAutoAllow("shell_execute")

	// 添加白名单后不再需要确认
	if m.ShouldApprove("shell_execute") {
		t.Errorf("after AddAutoAllow: ShouldApprove(shell_execute) = true, want false")
	}

	// 未知工具添加白名单
	if !m.ShouldApprove("custom_tool") {
		t.Errorf("before AddAutoAllow: ShouldApprove(custom_tool) = false, want true")
	}

	m.AddAutoAllow("custom_tool")

	if m.ShouldApprove("custom_tool") {
		t.Errorf("after AddAutoAllow: ShouldApprove(custom_tool) = true, want false")
	}
}

func TestManager_AddDeny(t *testing.T) {
	m := NewManager(ModeFullAuto)

	// 全自动模式下 read_file 自动放行
	if m.ShouldApprove("read_file") {
		t.Errorf("before AddDeny: ShouldApprove(read_file) = true, want false")
	}

	if m.IsDenied("read_file") {
		t.Errorf("before AddDeny: IsDenied(read_file) = true, want false")
	}

	m.AddDeny("read_file")

	// 添加黑名单后 IsDenied 返回 true，ShouldApprove 返回 false（不需要确认，直接拒绝）
	if !m.IsDenied("read_file") {
		t.Errorf("after AddDeny: IsDenied(read_file) = false, want true")
	}

	if m.ShouldApprove("read_file") {
		t.Errorf("after AddDeny: ShouldApprove(read_file) = true, want false (denied tool needs no confirmation)")
	}
}

func TestManager_DenyOverridesAllow(t *testing.T) {
	m := NewManager(ModeFullAuto)

	// 全自动模式下 shell_execute 自动放行
	if m.ShouldApprove("shell_execute") {
		t.Errorf("initial: ShouldApprove(shell_execute) = true, want false")
	}

	// 同时添加白名单和黑名单
	m.AddAutoAllow("shell_execute")
	m.AddDeny("shell_execute")

	// 黑名单优先：被拒绝的工具 IsDenied 返回 true，ShouldApprove 返回 false
	if !m.IsDenied("shell_execute") {
		t.Errorf("IsDenied(shell_execute) = false, want true")
	}

	if m.ShouldApprove("shell_execute") {
		t.Errorf("ShouldApprove(shell_execute) = true, want false (denied tool needs no confirmation)")
	}
}

func TestGetToolCategory(t *testing.T) {
	tests := []struct {
		tool string
		want string
	}{
		// 只读工具
		{"read_file", CategoryReadonly},
		{"list_dir", CategoryReadonly},
		{"grep_search", CategoryReadonly},
		{"glob_search", CategoryReadonly},
		{"web_request", CategoryReadonly},
		{"web_fetch", CategoryReadonly},
		// 编辑工具
		{"write_file", CategoryEdit},
		{"edit_file", CategoryEdit},
		// 危险工具
		{"shell_execute", CategoryDanger},
		// MCP 工具
		{"mcp_some_tool", CategoryMCP},
		{"mcp_another", CategoryMCP},
		{"mcp_", CategoryMCP},
		// 未知工具（当前实现中未知工具也归为 MCP 类别）
		{"unknown_tool", CategoryMCP},
		{"random", CategoryMCP},
	}

	for _, tt := range tests {
		got := GetToolCategory(tt.tool)
		if got != tt.want {
			t.Errorf("GetToolCategory(%q) = %q, want %q", tt.tool, got, tt.want)
		}
	}
}

func TestManager_ModeName(t *testing.T) {
	tests := []struct {
		mode ApprovalMode
		want string
	}{
		{ModeSuggest, "suggest"},
		{ModeAutoEdit, "auto-edit"},
		{ModeFullAuto, "full-auto"},
		{ApprovalMode(99), "unknown"},
	}

	for _, tt := range tests {
		m := NewManager(tt.mode)
		got := m.ModeName()
		if got != tt.want {
			t.Errorf("ModeName() for mode %v = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

// TestModeSwitchPreservesDenyList verifies that calling /mode (which
// used to do *m = *NewManager(newMode)) does not wipe the SafeMode
// denyList. F-02 regression guard.
func TestModeSwitchPreservesDenyList(t *testing.T) {
	m := NewManager(ModeSuggest)
	m.AddDeny("shell_execute")
	if !m.IsDenied("shell_execute") {
		t.Fatal("setup: shell_execute should be denied")
	}

	// simulate /mode full-auto via the public SetMode API
	m.SetMode(ModeFullAuto)

	if !m.IsDenied("shell_execute") {
		t.Errorf("BUG (F-02): SafeMode denyList wiped by mode switch")
	}

	// /mode auto-edit should not re-enable a denied tool either
	m.SetMode(ModeAutoEdit)
	if !m.IsDenied("shell_execute") {
		t.Errorf("BUG (F-02): SafeMode denyList wiped by auto-edit switch")
	}
}

// TestSetModePreservesAutoAllow verifies the user-built always-allow
// list survives a mode switch.
func TestSetModePreservesAutoAllow(t *testing.T) {
	m := NewManager(ModeSuggest)
	m.AddAutoAllow("read_file")
	if !m.IsAllowed("read_file") {
		t.Fatal("setup: read_file should be auto-allowed")
	}
	// switch to suggest: per NewManager, suggest has no default autoAllow,
	// but user-built entries should remain
	m.SetMode(ModeSuggest)
	if !m.IsAllowed("read_file") {
		t.Errorf("BUG: user-built autoAllow wiped by mode switch")
	}
}

// IsAllowed exposes whether the tool is auto-allowed (test helper).
func (m *Manager) IsAllowed(toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.autoAllow[toolName]
}
