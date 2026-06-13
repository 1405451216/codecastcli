package session

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB 打开一个临时 sqlite 库 + 建表，跑 List/Delete 等不需要真实业务数据的测试。
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS episodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		role TEXT,
		content TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestNewManagerSharesProvidedDB(t *testing.T) {
	db := openTestDB(t)
	mgr, err := NewManager(db)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr.ownsDB {
		t.Errorf("ownsDB should be false when db is provided")
	}
	if mgr.db != db {
		t.Errorf("mgr.db should equal provided db")
	}
	// 共享模式下 Close 不应关闭 db
	if err := mgr.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Errorf("shared db should still be alive after mgr.Close: %v", err)
	}
}

func TestNewManagerOpensOwnDB(t *testing.T) {
	// 不传 db：会走 config.GetConfigDir()，需要确保环境干净
	// 在 Windows CI 上 ~/.codecast 通常不存在或可写；遇到权限问题跳过。
	t.Skip("TestNewManagerOpensOwnDB touches user home directory; skipped in unit suite")
}

func TestListEmptyDB(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := NewManager(db)
	defer mgr.Close()

	sessions, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestGetHistoryEmptyDB(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := NewManager(db)
	defer mgr.Close()

	msgs, err := mgr.GetHistory("nonexistent", 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestDeleteNoopOnEmpty(t *testing.T) {
	db := openTestDB(t)
	mgr, _ := NewManager(db)
	defer mgr.Close()

	if err := mgr.Delete("nonexistent"); err != nil {
		t.Errorf("Delete on nonexistent: %v", err)
	}
}

// TestNewManagerFallsBackToSharedDB 验证 F-05 进程级共享 DB 注入路径。
func TestNewManagerFallsBackToSharedDB(t *testing.T) {
	db := openTestDB(t)
	// 清理全局状态
	orig := GetSharedDB()
	t.Cleanup(func() { SetSharedDB(orig) })
	SetSharedDB(db)

	mgr, err := NewManager() // 不传参数，应自动用 sharedDB
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr.ownsDB {
		t.Errorf("fallback to sharedDB should set ownsDB=false")
	}
	if mgr.db != db {
		t.Errorf("mgr.db should equal the shared db")
	}
	// 共享模式下 Close 不应关闭 db
	if err := mgr.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Errorf("shared db should still be alive after mgr.Close: %v", err)
	}
}

// TestSetSharedDBOverride 显式参数优先于 sharedDB。
func TestSetSharedDBOverride(t *testing.T) {
	sharedDB := openTestDB(t)
	explicit := openTestDB(t)

	orig := GetSharedDB()
	t.Cleanup(func() { SetSharedDB(orig) })
	SetSharedDB(sharedDB)

	mgr, err := NewManager(explicit) // 显式 > shared
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr.db == sharedDB {
		t.Errorf("explicit arg should take precedence over sharedDB")
	}
}
