package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// SelfUpdate downloads the latest release for the current platform,
// validates checksum, replaces the binary atomically, and prints the result.
// Returns nil on success or if already up to date.
func SelfUpdate(currentVersion, execPath string) error {
	// Step 1: check write permission
	if err := checkWritePermission(execPath); err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Step 2: fetch latest release info
	release, err := fetchLatestRelease(client, currentVersion)
	if err != nil {
		return fmt.Errorf("failed to check latest version: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	// Step 3: compare versions
	if semver.Compare("v"+currentVersion, "v"+latestVersion) >= 0 {
		fmt.Printf("aimux v%s is already up to date.\n", currentVersion)
		return nil
	}

	// Step 4: build archive name (zip for Windows, tar.gz for everything else)
	archiveExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		archiveExt = ".zip"
	}
	archiveName := fmt.Sprintf("aimux_%s_%s_%s%s", latestVersion, runtime.GOOS, runtime.GOARCH, archiveExt)

	// Step 5: find matching asset
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == archiveName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Step 6: download archive to temp file
	tmpPattern := "aimux_download_*.tar.gz"
	if runtime.GOOS == "windows" {
		tmpPattern = "aimux_download_*.zip"
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), tmpPattern)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	resp, err := client.Get(downloadURL)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		resp.Body.Close()
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		resp.Body.Close()
		return fmt.Errorf("download write failed: %w", err)
	}
	tmpFile.Close()
	resp.Body.Close()

	// Step 7: download checksums
	var checksumsURL string
	for _, asset := range release.Assets {
		if asset.Name == "checksums.txt" {
			checksumsURL = asset.BrowserDownloadURL
			break
		}
	}

	if checksumsURL == "" {
		return fmt.Errorf("checksums.txt not found in release assets")
	}

	checksumsResp, err := client.Get(checksumsURL)
	if err != nil {
		return fmt.Errorf("download checksums failed: %w", err)
	}
	defer checksumsResp.Body.Close()

	if checksumsResp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksums download returned %d", checksumsResp.StatusCode)
	}

	checksumsBytes, err := io.ReadAll(checksumsResp.Body)
	if err != nil {
		return fmt.Errorf("read checksums failed: %w", err)
	}

	// Find matching checksum for the archive
	var expectedSHA string
	for _, line := range strings.Split(string(checksumsBytes), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == archiveName {
			expectedSHA = parts[0]
			break
		}
	}
	if expectedSHA == "" {
		return fmt.Errorf("checksum not found for %s", archiveName)
	}

	// Step 8: validate archive checksum before extraction
	if err := validateChecksum(tmpPath, expectedSHA); err != nil {
		return err
	}

	// Step 9: extract binary from archive
	extractedPath := filepath.Join(filepath.Dir(execPath), ".aimux_extracted_"+randomString(8))
	defer os.Remove(extractedPath)

	if err := extractBinary(tmpPath, extractedPath); err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}

	// Step 10: atomic replace
	if err := atomicReplace(extractedPath, execPath); err != nil {
		return err
	}

	fmt.Printf("aimux updated from v%s to v%s. Please restart aimux.\n", currentVersion, latestVersion)
	return nil
}

// isHomebrewInstall checks if the binary is installed via Homebrew.
func isHomebrewInstall(execPath string) bool {
	cmd := exec.Command("brew", "--prefix")
	output, err := cmd.Output()
	if err != nil {
		return false // brew not available
	}
	brewPrefix := strings.TrimSpace(string(output))
	return strings.HasPrefix(execPath, brewPrefix)
}

// isHomebrewInstallExported is the exported wrapper for use from main.go.
func IsHomebrewInstall(execPath string) bool {
	return isHomebrewInstall(execPath)
}

// homebrewUpdate runs brew upgrade for the aimux tap.
func homebrewUpdate() int {
	cmd := exec.Command("brew", "upgrade", "MileniumTick/tap/aimux")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: brew upgrade failed: %v\n", err)
		return 1
	}
	fmt.Println("aimux updated successfully via Homebrew.")
	return 0
}

// HomebrewUpdate is the exported wrapper for use from main.go.
func HomebrewUpdate() int {
	return homebrewUpdate()
}

// checkWritePermission tests write access by creating a temp file in the binary's directory.
func checkWritePermission(execPath string) error {
	dir := filepath.Dir(execPath)
	testFile := filepath.Join(dir, ".aimux_update_test_"+randomString(6))
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("no write permission in %s: %w\nTry: sudo aimux update", dir, err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// validateChecksum computes SHA256 of a file and compares it to the expected hex string.
func validateChecksum(filePath, expectedSHA256 string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("compute checksum: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expectedSHA256) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA256, actual)
	}
	return nil
}

// atomicReplace atomically replaces the target binary with a verified temp file.
func atomicReplace(verifiedPath, targetPath string) error {
	tmpPath := targetPath + ".tmp." + randomString(8)

	if err := os.Rename(verifiedPath, tmpPath); err != nil {
		return fmt.Errorf("move to temp: %w", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename over target: %w", err)
	}

	return nil
}

// extractBinary extracts the binary from a gzipped tar archive or zip archive.
// Dispatches to extractTarGz or extractZip based on file extension.
func extractBinary(archivePath, destPath string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destPath)
	}
	return extractTarGz(archivePath, destPath)
}

// extractTarGz extracts the binary from a gzipped tar archive.
func extractTarGz(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeReg && (header.Name == "aimux" || header.Name == "aimux.exe") {
			out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			defer out.Close()

			if _, err := io.Copy(out, tr); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("binary not found in archive")
}

// extractZip extracts the binary from a zip archive.
func extractZip(archivePath, destPath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == "aimux.exe" || f.Name == "aimux" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			defer out.Close()

			if _, err := io.Copy(out, rc); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("binary not found in zip archive")
}

// randomString generates a random alphanumeric string of the given length.
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use sequential bytes (crypto/rand should always succeed on modern OS)
		for i := range b {
			b[i] = letters[i%len(letters)]
		}
		return string(b)
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}
