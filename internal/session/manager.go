package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codecast/cli/internal/config"
	_ "modernc.org/sqlite"
)

// Info 会话信息
type Info struct {
	SessionID   string    `json:"session_id"`
	Name        string    `json:"name"`
	MessageCount int      `json:"message_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Manager 会话管理器
type Manager struct {
	db     *sql.DB
	ownsDB bool // 若为 true，Close 时关闭 db；否则只是解引用（F-05）
}

// NewManager 创建会话管理器。
// 优先使用显式传入的 db；为空时回退到 GetSharedDB() 注入的共享连接；
// 仍为空时自行打开 ~/.codecast/memory.db。
// 这样的三段式让 agent 在启动时一次性 SetSharedDB，下游调用方可零参数使用。
func NewManager(db ...*sql.DB) (*Manager, error) {
	var explicit *sql.DB
	if len(db) > 0 {
		explicit = db[0]
	}

	var mgr *Manager
	switch {
	case explicit != nil:
		mgr = &Manager{db: explicit, ownsDB: false}
	default:
		// 回退到 SetSharedDB 注入的进程级共享连接
		if shared := GetSharedDB(); shared != nil {
			mgr = &Manager{db: shared, ownsDB: false}
			break
		}
		configDir := config.GetConfigDir()
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return nil, fmt.Errorf("创建配置目录失败: %w", err)
		}
		dbPath := filepath.Join(configDir, "memory.db")
		opened, err := sql.Open("sqlite", dbPath)
		if err != nil {
			return nil, fmt.Errorf("打开记忆数据库失败: %w", err)
		}
		mgr = &Manager{db: opened, ownsDB: true}
	}

	// R5-C7 修复：确保 episodes 表存在（session.Manager 独立使用时，AgentPrimordia 可能尚未创建）
	if err := mgr.ensureSchema(); err != nil {
		if mgr.ownsDB {
			mgr.db.Close()
		}
		return nil, fmt.Errorf("初始化会话表失败: %w", err)
	}
	return mgr, nil
}

// ensureSchema 确保 episodes 表存在
func (m *Manager) ensureSchema() error {
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS episodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		metadata TEXT DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	return err
}

// List 返回所有会话列表
func (m *Manager) List() ([]Info, error) {
	rows, err := m.db.Query(`
		SELECT session_id,
			COUNT(*) as msg_count,
			MIN(created_at) as created_at,
			MAX(created_at) as updated_at
		FROM episodes
		WHERE session_id IS NOT NULL AND session_id != ''
		GROUP BY session_id
		ORDER BY updated_at DESC
		LIMIT 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Info
	for rows.Next() {
		var s Info
		var createdStr, updatedStr string
		if err := rows.Scan(&s.SessionID, &s.MessageCount, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		// 使用 session_id 作为默认名称
		s.Name = s.SessionID
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetHistory 获取指定会话的历史消息
func (m *Manager) GetHistory(sessionID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := m.db.Query(`
		SELECT role, content, created_at
		FROM episodes
		WHERE session_id = ?
		ORDER BY created_at ASC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var msg Message
		var createdStr string
		if err := rows.Scan(&msg.Role, &msg.Content, &createdStr); err != nil {
			return nil, err
		}
		msg.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

// Delete 删除指定会话
func (m *Manager) Delete(sessionID string) error {
	_, err := m.db.Exec(`DELETE FROM episodes WHERE session_id = ?`, sessionID)
	return err
}

// Close 关闭数据库。若 ownsDB 为 false（共享模式），仅解引用。
func (m *Manager) Close() error {
	if m == nil || m.db == nil {
		return nil
	}
	if !m.ownsDB {
		m.db = nil
		return nil
	}
	err := m.db.Close()
	m.db = nil
	return err
}

// Message 会话消息
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// sharedDB 进程级共享 DB 连接（由 agent 在启动时通过 SetSharedDB 注入）。
// 仅作 fallback 路径：当调用方没有显式传入 db 时，GetSharedDB 返回该连接。
var sharedDB *sql.DB

// SetSharedDB 注入进程级共享 SQLite 连接，供未显式传入 db 的调用方使用。
// 通常在 CodecastAgent 启动时（newAgent）调用一次。
// 传 nil 可清除旧引用（用于测试/重置）。
func SetSharedDB(db *sql.DB) {
	sharedDB = db
}

// GetSharedDB 返回通过 SetSharedDB 注入的共享连接，未注入时返回 nil。
func GetSharedDB() *sql.DB {
	return sharedDB
}
