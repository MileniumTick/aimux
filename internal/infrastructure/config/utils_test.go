package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempBackupRoot redirects the centralized backup store to a temp dir for
// the duration of a test so it never touches the real ~/.config/aimux/backups.
func withTempBackupRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "backups")
	orig := backupRootFn
	backupRootFn = func() (string, error) { return root, nil }
	t.Cleanup(func() { backupRootFn = orig })
	return root
}

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
	withTempBackupRoot(t)
	srcDir := t.TempDir()
	path := filepath.Join(srcDir, "settings.json")

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

	// Backup must live in the centralized store, NOT next to the source file.
	if filepath.Dir(backupPath) == srcDir {
		t.Errorf("backup should be centralized, not next to source: %s", backupPath)
	}
}

func TestCreateBackup_NonExistentFile(t *testing.T) {
	withTempBackupRoot(t)
	path := filepath.Join(t.TempDir(), "nonexistent.json")

	_, err := CreateBackup(path)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestRestoreBackup_RoundTrip(t *testing.T) {
	withTempBackupRoot(t)
	srcDir := t.TempDir()
	path := filepath.Join(srcDir, "settings.json")

	original := `{"data": "original"}`
	os.WriteFile(path, []byte(original), 0644)

	bp, err := CreateBackup(path)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Mutate the source, then restore from backup.
	os.WriteFile(path, []byte(`{"data": "mutated"}`), 0644)
	if err := RestoreBackup(bp, path); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Errorf("expected restored content %q, got %q", original, string(got))
	}
}

func TestPruneBackups_RemovesExcess(t *testing.T) {
	withTempBackupRoot(t)
	src := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(src, []byte(`{}`), 0644)

	dir, err := backupDirFor(src)
	if err != nil {
		t.Fatalf("backupDirFor failed: %v", err)
	}
	os.MkdirAll(dir, 0700)

	base := filepath.Base(src)
	for i := 0; i < 7; i++ {
		ts := fmt.Sprintf("2024-01-0%dT00-00-00Z", i+1)
		os.WriteFile(filepath.Join(dir, base+"."+ts), []byte(`{}`), 0644)
	}

	PruneBackups(src, 5)

	backups, _ := ListBackups(src)
	if len(backups) > 5 {
		t.Errorf("expected at most 5 backups, got %d", len(backups))
	}
}

func TestPruneBackups_UnderLimit(t *testing.T) {
	withTempBackupRoot(t)
	src := filepath.Join(t.TempDir(), "app.json")
	os.WriteFile(src, []byte(`{}`), 0644)

	dir, err := backupDirFor(src)
	if err != nil {
		t.Fatalf("backupDirFor failed: %v", err)
	}
	os.MkdirAll(dir, 0700)

	base := filepath.Base(src)
	for i := 0; i < 2; i++ {
		ts := fmt.Sprintf("2024-01-0%dT00-00-00Z", i+1)
		os.WriteFile(filepath.Join(dir, base+"."+ts), []byte(`{}`), 0644)
	}

	PruneBackups(src, 5)

	backups, _ := ListBackups(src)
	if len(backups) != 2 {
		t.Errorf("expected 2 backups (under limit), got %d", len(backups))
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
