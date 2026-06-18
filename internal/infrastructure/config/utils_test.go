package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJSONWithLock_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	content := `{"key": "value", "number": 42}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	result, err := ReadJSONWithLock(path)
	if err != nil {
		t.Fatalf("ReadJSONWithLock failed: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected 'value', got %v", result["key"])
	}
	if result["number"] != float64(42) {
		t.Errorf("expected 42, got %v", result["number"])
	}
}

func TestReadJSONWithLock_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	result, err := ReadJSONWithLock(path)
	if err != nil {
		t.Fatalf("ReadJSONWithLock on empty file should not error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestReadJSONWithLock_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	result, err := ReadJSONWithLock(path)
	if err != nil {
		t.Fatalf("ReadJSONWithLock on non-existent file should not error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestReadJSONWithLock_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")

	content := `{invalid json here}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	result, err := ReadJSONWithLock(path)
	if err == nil {
		t.Fatal("ReadJSONWithLock on invalid JSON should return an error")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map for invalid JSON, got %v", result)
	}
}

func TestAtomicWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	data := []byte(`{"hello": "world"}` + "\n")
	if err := AtomicWrite(data, path); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != string(data) {
		t.Errorf("expected %q, got %q", string(data), string(content))
	}

	// No temp files should remain
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("stale temp file found: %s", e.Name())
		}
	}
}

func TestAtomicWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	os.WriteFile(path, []byte(`"old"`), 0644)

	data := []byte(`{"new": "data"}` + "\n")
	if err := AtomicWrite(data, path); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != string(data) {
		t.Errorf("expected %q, got %q", string(data), string(content))
	}
}

func TestWriteAtomicJSON_ValidData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	data := map[string]any{
		"key":   "value",
		"count": float64(42),
	}

	if err := WriteAtomicJSON(path, data); err != nil {
		t.Fatalf("WriteAtomicJSON failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var result map[string]any
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected 'value', got %v", result["key"])
	}
}

func TestCreateBackup_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	original := `{"data": "original"}`
	os.WriteFile(path, []byte(original), 0644)

	backupPath, err := CreateBackup(path)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatalf("backup file does not exist: %s", backupPath)
	}

	backupContent, _ := os.ReadFile(backupPath)
	if string(backupContent) != original {
		t.Errorf("backup should match original, got %q", string(backupContent))
	}

	if !strings.HasPrefix(filepath.Base(backupPath), "settings.json.aimux-backup-") {
		t.Errorf("unexpected backup filename: %s", filepath.Base(backupPath))
	}
}

func TestCreateBackup_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	_, err := CreateBackup(path)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestPruneBackups_RemovesExcess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{}`), 0644)

	// Create 7 backup files (older names sort first, so we create them in reverse)
	for i := 0; i < 7; i++ {
		// Create with slightly different timestamps
		backupName := "config.json.aimux-backup-2024-01-0" + string(rune('1'+i)) + "T00:00:00Z"
		os.WriteFile(filepath.Join(dir, backupName), []byte(`{}`), 0644)
	}

	PruneBackups(path, 5)

	entries, _ := os.ReadDir(dir)
	backupCount := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config.json.aimux-backup-") {
			backupCount++
		}
	}

	if backupCount > 5 {
		t.Errorf("expected at most 5 backups, got %d", backupCount)
	}
}

func TestPruneBackups_UnderLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.json")
	os.WriteFile(path, []byte(`{}`), 0644)

	// Create 2 backup files
	for i := 0; i < 2; i++ {
		backupName := "app.json.aimux-backup-2024-01-0" + string(rune('1'+i)) + "T00:00:00Z"
		os.WriteFile(filepath.Join(dir, backupName), []byte(`{}`), 0644)
	}

	PruneBackups(path, 5)

	entries, _ := os.ReadDir(dir)
	backupCount := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "app.json.aimux-backup-") {
			backupCount++
		}
	}

	if backupCount != 2 {
		t.Errorf("expected 2 backups (under limit), got %d", backupCount)
	}
}

func TestPrepareDir_CreatesNested(t *testing.T) {
	dir := t.TempDir()
	nestedPath := filepath.Join(dir, "a", "b", "c", "file.json")

	if err := PrepareDir(nestedPath); err != nil {
		t.Fatalf("PrepareDir failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "a", "b", "c")); os.IsNotExist(err) {
		t.Error("expected nested directory to be created")
	}
}

func TestPrepareDir_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.json")

	// First call creates
	if err := PrepareDir(path); err != nil {
		t.Fatalf("first PrepareDir failed: %v", err)
	}

	// Second call should not error
	if err := PrepareDir(path); err != nil {
		t.Fatalf("second PrepareDir failed: %v", err)
	}
}
