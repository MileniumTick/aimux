package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckWritePermission_WritableDir(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "aimux")

	f, err := os.Create(execPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	err = checkWritePermission(execPath)
	if err != nil {
		t.Errorf("checkWritePermission on writable dir should succeed: %v", err)
	}
}

func TestCheckWritePermission_NonWritableDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping permission test in short mode")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0400); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)

	execPath := filepath.Join(dir, "aimux")
	err := checkWritePermission(execPath)
	if err == nil {
		t.Errorf("checkWritePermission on non-writable dir should fail")
	}
}

func TestValidateChecksum_Match(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world")
	filePath := filepath.Join(dir, "test.bin")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	expectedSHA := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expectedSHA[:])

	err := validateChecksum(filePath, expectedHex)
	if err != nil {
		t.Errorf("validateChecksum should pass for matching checksum: %v", err)
	}
}

func TestValidateChecksum_Mismatch(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world")
	filePath := filepath.Join(dir, "test.bin")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	err := validateChecksum(filePath, "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Errorf("validateChecksum should fail for mismatched checksum")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error should contain 'checksum mismatch', got: %v", err)
	}
}

func TestValidateChecksum_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "empty.bin")

	if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	expectedSHA := sha256.Sum256([]byte{})
	expectedHex := hex.EncodeToString(expectedSHA[:])

	err := validateChecksum(filePath, expectedHex)
	if err != nil {
		t.Errorf("validateChecksum should pass for empty file with matching checksum: %v", err)
	}
}

func TestValidateChecksum_NonexistentFile(t *testing.T) {
	err := validateChecksum("/nonexistent/path/file.bin", "0000")
	if err == nil {
		t.Errorf("validateChecksum should fail for nonexistent file")
	}
}

func TestAtomicReplace_NewTarget(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.bin")
	targetPath := filepath.Join(dir, "target.bin")

	srcContent := []byte("verified content")
	if err := os.WriteFile(srcPath, srcContent, 0644); err != nil {
		t.Fatal(err)
	}

	err := atomicReplace(srcPath, targetPath)
	if err != nil {
		t.Fatalf("atomicReplace failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(srcContent) {
		t.Errorf("target content = %q, want %q", string(data), string(srcContent))
	}

	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Errorf("source temp file should be removed after atomicReplace")
	}
}

func TestAtomicReplace_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.bin")
	targetPath := filepath.Join(dir, "target.bin")

	if err := os.WriteFile(targetPath, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	srcContent := []byte("new content")
	if err := os.WriteFile(srcPath, srcContent, 0644); err != nil {
		t.Fatal(err)
	}

	err := atomicReplace(srcPath, targetPath)
	if err != nil {
		t.Fatalf("atomicReplace failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(srcContent) {
		t.Errorf("target content = %q, want %q", string(data), string(srcContent))
	}
}

func TestAtomicReplace_SameFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.bin")
	content := []byte("same file content")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	err := atomicReplace(filePath, filePath)
	if err != nil {
		t.Logf("atomicReplace(same, same) returned: %v (acceptable)", err)
	}
}

func TestRandomString_LengthAndCharset(t *testing.T) {
	tests := []struct {
		name string
		n    int
	}{
		{"zero", 0},
		{"one", 1},
		{"five", 5},
		{"ten", 10},
		{"hundred", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := randomString(tt.n)
			if len(s) != tt.n {
				t.Errorf("randomString(%d) length = %d, want %d", tt.n, len(s), tt.n)
			}

			// Verify charset: only lowercase letters and digits
			for _, ch := range s {
				if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
					t.Errorf("randomString contains invalid char %c (code %d)", ch, ch)
				}
			}
		})
	}
}

func TestExtractTarGz_Success(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "archive.tar.gz")
	destPath := filepath.Join(dir, "extracted_binary")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	tarContent := []byte("#!/bin/bash\necho 'aimux binary'")
	hdr := &tar.Header{
		Name: "aimux",
		Size: int64(len(tarContent)),
		Mode: 0755,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(tarContent); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()

	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	err := extractTarGz(archivePath, destPath)
	if err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(tarContent) {
		t.Errorf("extracted content = %q, want %q", string(data), string(tarContent))
	}
}

func TestExtractTarGz_NoBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "archive.tar.gz")
	destPath := filepath.Join(dir, "extracted")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name: "readme.txt",
		Size: int64(5),
		Mode: 0644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()

	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	err := extractTarGz(archivePath, destPath)
	if err == nil {
		t.Errorf("extractTarGz should fail when binary not found")
	}
}

func TestExtractTarGz_Corrupted(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "corrupted.tar.gz")
	destPath := filepath.Join(dir, "extracted")

	if err := os.WriteFile(archivePath, []byte("not a valid gzip"), 0644); err != nil {
		t.Fatal(err)
	}

	err := extractTarGz(archivePath, destPath)
	if err == nil {
		t.Errorf("extractTarGz should fail on corrupted archive")
	}
}

func TestExtractZip_Success(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "archive.zip")
	destPath := filepath.Join(dir, "extracted_binary")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	zipContent := []byte("zip binary content")
	fw, err := zw.Create("aimux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(zipContent); err != nil {
		t.Fatal(err)
	}
	zw.Close()

	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	err = extractZip(archivePath, destPath)
	if err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(zipContent) {
		t.Errorf("extracted content = %q, want %q", string(data), string(zipContent))
	}
}

func TestExtractZip_NoBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "archive.zip")
	destPath := filepath.Join(dir, "extracted")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fw, err := zw.Create("readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	zw.Close()

	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	err = extractZip(archivePath, destPath)
	if err == nil {
		t.Errorf("extractZip should fail when binary not found")
	}
}

func TestExtractZip_Corrupted(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "corrupted.zip")
	destPath := filepath.Join(dir, "extracted")

	if err := os.WriteFile(archivePath, []byte("not a valid zip"), 0644); err != nil {
		t.Fatal(err)
	}

	err := extractZip(archivePath, destPath)
	if err == nil {
		t.Errorf("extractZip should fail on corrupted archive")
	}
}

func TestExtractBinary_TarGz(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "aimux_linux_amd64.tar.gz")
	destPath := filepath.Join(dir, "extracted")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte("binary from tar.gz")
	hdr := &tar.Header{Name: "aimux", Size: int64(len(content)), Mode: 0755}
	tw.WriteHeader(hdr)
	tw.Write(content)
	tw.Close()
	gz.Close()

	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	err := extractBinary(archivePath, destPath)
	if err != nil {
		t.Fatalf("extractBinary for tar.gz failed: %v", err)
	}
	data, _ := os.ReadFile(destPath)
	if string(data) != string(content) {
		t.Errorf("extracted content = %q, want %q", string(data), string(content))
	}
}

func TestExtractBinary_Zip(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "aimux_windows_amd64.zip")
	destPath := filepath.Join(dir, "extracted")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	content := []byte("binary from zip")
	fw, _ := zw.Create("aimux.exe")
	fw.Write(content)
	zw.Close()

	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	err := extractBinary(archivePath, destPath)
	if err != nil {
		t.Fatalf("extractBinary for zip failed: %v", err)
	}
	data, _ := os.ReadFile(destPath)
	if string(data) != string(content) {
		t.Errorf("extracted content = %q, want %q", string(data), string(content))
	}
}

func TestExtractBinary_UnknownSuffix(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "aimux.unknown")
	destPath := filepath.Join(dir, "extracted")

	if err := os.WriteFile(archivePath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	err := extractBinary(archivePath, destPath)
	if err == nil {
		t.Errorf("extractBinary should fail for unknown suffix")
	}
}

func TestExtractBinary_NonexistentFile(t *testing.T) {
	err := extractBinary("/nonexistent/archive.tar.gz", "/dev/null")
	if err == nil {
		t.Errorf("extractBinary should fail for nonexistent file")
	}
}

func TestIsHomebrewInstall_SkipInShort(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Homebrew test in short mode")
	}
	_ = IsHomebrewInstall("/usr/local/bin/aimux")
}

func TestHomebrewUpdate_SkipInShort(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Homebrew test in short mode")
	}
	_ = HomebrewUpdate()
}
