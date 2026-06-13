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

// sharedDB 进程级的共享 SQLite 连接（F-05 配套）。
// 由 agent.New() 在启动时设置；其他模块（session.Manager 等）
// 在 NewManager() 中自动复用。
//
// 注意：这只是"尽力而为"的共享；如果 agent 没启动过，sharedDB 仍为 nil。
var sharedDB *sql.DB

// SetSharedDB 注入共享 DB（F-05）。通常由 agent.New() 在初始化时调用。
func SetSharedDB(db *sql.DB) {
	sharedDB = db
}

// GetSharedDB 返回当前注入的共享 DB，可能为 nil。
func GetSharedDB() *sql.DB {
	return sharedDB
}

// NewManager 创建会话管理器。
// 若显式传入 db != nil，优先使用调用方的连接；
// 否则退回到进程级 sharedDB（F-05）；
// 都没有时才自己打开 ~/.codecast/memory.db。
func NewManager(db ...*sql.DB) (*Manager, error) {
	// 优先级：显式参数 > 进程级共享 > 自开
	if len(db) > 0 && db[0] != nil {
		return &Manager{db: db[0], ownsDB: false}, nil
	}
	if sharedDB != nil {
		return &Manager{db: sharedDB, ownsDB: false}, nil
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
	return &Manager{db: opened, ownsDB: true}, nil
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
