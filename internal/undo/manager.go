package undo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// BackupEntry represents a single backup file record.
type BackupEntry struct {
	OriginalPath string
	BackupPath   string
	Timestamp    time.Time
}

// Manager manages file backups for undo/rollback operations.
type Manager struct {
	backupDir   string
	maxBackups  int
	mu          sync.Mutex
}

// NewManager creates a new Manager with backupDir set to .codecast/backups
// under the given projectRoot.
func NewManager(projectRoot string) *Manager {
	return &Manager{
		backupDir:  filepath.Join(projectRoot, ".codecast", "backups"),
		maxBackups: 50,
	}
}

// Backup reads the file at filePath and saves a timestamped copy to backupDir.
// The backup filename format is: filename_20060102_150405.bak
// If the number of backups for this file exceeds maxBackups, the oldest are removed.
func (m *Manager) Backup(filePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file %s: %w", filePath, err)
	}

	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	ext := filepath.Ext(base)
	timestamp := time.Now().Format("20060102_150405")
	backupName := fmt.Sprintf("%s_%s%s.bak", name, timestamp, ext)
	backupPath := filepath.Join(m.backupDir, backupName)

	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return fmt.Errorf("write backup %s: %w", backupPath, err)
	}

	if err := m.enforceMaxBackups(base); err != nil {
		// Non-fatal: backup was created, but cleanup failed.
		_ = err
	}

	return nil
}

// enforceMaxBackups removes the oldest backups for a given original filename
// until only maxBackups remain. Must be called with m.mu held.
func (m *Manager) enforceMaxBackups(originalBase string) error {
	entries, err := m.backupsForFile(originalBase)
	if err != nil {
		return err
	}

	if len(entries) <= m.maxBackups {
		return nil
	}

	// Sort oldest first so we remove from the beginning.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	toRemove := len(entries) - m.maxBackups
	for i := 0; i < toRemove; i++ {
		if err := os.Remove(entries[i].BackupPath); err != nil {
			return fmt.Errorf("remove old backup %s: %w", entries[i].BackupPath, err)
		}
	}
	return nil
}

// Restore finds the most recent backup for the file at filePath and copies it
// back. Returns true if a backup was found and restored, false if no backup exists.
func (m *Manager) Restore(filePath string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	base := filepath.Base(filePath)
	entries, err := m.backupsForFile(base)
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return false, nil
	}

	// Sort newest first.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	latest := entries[0]
	data, err := os.ReadFile(latest.BackupPath)
	if err != nil {
		return false, fmt.Errorf("read backup %s: %w", latest.BackupPath, err)
	}

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return false, fmt.Errorf("restore file %s: %w", filePath, err)
	}

	return true, nil
}

// ListBackups returns all backup entries sorted by timestamp (newest first).
func (m *Manager) ListBackups() []BackupEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := m.allBackups()
	if err != nil {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	return entries
}

// Cleanup removes backups older than 24 hours.
func (m *Manager) Cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := m.allBackups()
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	var firstErr error
	for _, e := range entries {
		if e.Timestamp.Before(cutoff) {
			if err := os.Remove(e.BackupPath); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("remove backup %s: %w", e.BackupPath, err)
			}
		}
	}
	return firstErr
}

// backupsForFile returns backup entries that belong to a specific original filename.
// Must be called with m.mu held.
func (m *Manager) backupsForFile(originalBase string) ([]BackupEntry, error) {
	name := strings.TrimSuffix(originalBase, filepath.Ext(originalBase))
	ext := filepath.Ext(originalBase)
	// Backup files match pattern: name_20060102_150405.ext.bak
	prefix := name + "_"
	suffix := ext + ".bak"

	entries, err := m.allBackups()
	if err != nil {
		return nil, err
	}

	var matched []BackupEntry
	for _, e := range entries {
		base := filepath.Base(e.BackupPath)
		if strings.HasPrefix(base, prefix) && strings.HasSuffix(base, suffix) {
			matched = append(matched, e)
		}
	}
	return matched, nil
}

// allBackups reads the backup directory and returns all backup entries.
// Must be called with m.mu held.
func (m *Manager) allBackups() ([]BackupEntry, error) {
	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("ensure backup directory: %w", err)
	}

	infos, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, fmt.Errorf("read backup directory: %w", err)
	}

	var entries []BackupEntry
	for _, info := range infos {
		if info.IsDir() {
			continue
		}
		if !strings.HasSuffix(info.Name(), ".bak") {
			continue
		}

		ts, origBase := parseBackupName(info.Name())
		if ts.IsZero() {
			continue
		}

		entries = append(entries, BackupEntry{
			OriginalPath: origBase,
			BackupPath:   filepath.Join(m.backupDir, info.Name()),
			Timestamp:    ts,
		})
	}
	return entries, nil
}

// parseBackupName extracts the timestamp and original base filename from a
// backup filename. Expected format: name_20060102_150405.ext.bak
func parseBackupName(backupName string) (time.Time, string) {
	// Remove .bak suffix
	name := strings.TrimSuffix(backupName, ".bak")

	// Find the last extension (e.g. ".go", ".txt")
	ext := filepath.Ext(name)
	if ext == "" {
		return time.Time{}, ""
	}

	// The part before the extension contains name_YYYYMMDD_HHMMSS
	prefix := strings.TrimSuffix(name, ext)

	// Find the two last underscores to extract _YYYYMMDD_HHMMSS
	lastUnderscore := strings.LastIndex(prefix, "_")
	if lastUnderscore < 0 {
		return time.Time{}, ""
	}

	secondLastUnderscore := strings.LastIndex(prefix[:lastUnderscore], "_")
	if secondLastUnderscore < 0 {
		return time.Time{}, ""
	}

	datePart := prefix[secondLastUnderscore+1 : lastUnderscore]
	timePart := prefix[lastUnderscore+1:]
	tsStr := datePart + "_" + timePart

	ts, err := time.ParseInLocation("20060102_150405", tsStr, time.Local)
	if err != nil {
		return time.Time{}, ""
	}

	origBase := prefix[:secondLastUnderscore] + ext
	return ts, origBase
}
