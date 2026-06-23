package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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

	apiClient := &http.Client{Timeout: 10 * time.Second}
	dlClient := &http.Client{}

	// Step 2: fetch latest release info
	release, err := fetchLatestRelease(apiClient, currentVersion)
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

	dlCtx, dlCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer dlCancel()

	if err := downloadWithRetry(dlCtx, dlClient, downloadURL, tmpFile); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	tmpFile.Close()

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

	checksumsCtx, checksumsCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer checksumsCancel()

	checksumsReq, err := http.NewRequestWithContext(checksumsCtx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return fmt.Errorf("create checksums request: %w", err)
	}
	checksumsResp, err := dlClient.Do(checksumsReq)
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

// IsHomebrewInstall checks if the binary is installed via Homebrew.
func IsHomebrewInstall(execPath string) bool {
	cmd := exec.Command("brew", "--prefix")
	output, err := cmd.Output()
	if err != nil {
		return false // brew not available
	}
	brewPrefix := strings.TrimSpace(string(output))
	return strings.HasPrefix(execPath, brewPrefix)
}

// HomebrewUpdate runs brew upgrade for the aimux tap.
func HomebrewUpdate() int {
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

// downloadWithRetry downloads a URL into a writer with retry on transient errors.
func downloadWithRetry(ctx context.Context, client *http.Client, url string, w io.Writer) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
			if f, ok := w.(*os.File); ok {
				f.Truncate(0)
				f.Seek(0, 0)
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if isRetryable(err) {
				continue
			}
			return err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("download returned %d", resp.StatusCode)
		}

		_, err = io.Copy(w, resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			if isRetryable(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("download failed after 3 attempts: %w", lastErr)
}

// isRetryable returns true if the error is a net.Timeout or context deadline exceeded.
func isRetryable(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// StageUpdate checks for a newer version and downloads the binary
// to ~/.config/aimux/.staged-update/aimux. Metadata is written to
// ~/.config/aimux/.staged-update/metadata.json.
// Returns UpdateInfo{HasUpdate: false} on any error (never crashes).
func StageUpdate(currentVersion string) *UpdateInfo {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return &UpdateInfo{HasUpdate: false}
	}
	stageDir := filepath.Join(configDir, "aimux", ".staged-update")
	stageBin := filepath.Join(stageDir, "aimux")

	apiClient := &http.Client{Timeout: 10 * time.Second}
	release, err := fetchLatestRelease(apiClient, currentVersion)
	if err != nil {
		return &UpdateInfo{HasUpdate: false}
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	if semver.Compare("v"+currentVersion, "v"+latestVersion) >= 0 {
		return &UpdateInfo{HasUpdate: false}
	}

	archiveExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		archiveExt = ".zip"
	}
	archiveName := fmt.Sprintf("aimux_%s_%s_%s%s", latestVersion, runtime.GOOS, runtime.GOARCH, archiveExt)

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == archiveName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return &UpdateInfo{HasUpdate: false}
	}

	tmpFile, err := os.CreateTemp("", "aimux_stage_*"+archiveExt)
	if err != nil {
		return &UpdateInfo{HasUpdate: false}
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	dlCtx, dlCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer dlCancel()
	dlClient := &http.Client{}

	if err := downloadWithRetry(dlCtx, dlClient, downloadURL, tmpFile); err != nil {
		tmpFile.Close()
		return &UpdateInfo{HasUpdate: false}
	}
	tmpFile.Close()

	var checksumsURL string
	for _, asset := range release.Assets {
		if asset.Name == "checksums.txt" {
			checksumsURL = asset.BrowserDownloadURL
			break
		}
	}

	if checksumsURL == "" {
		return &UpdateInfo{HasUpdate: false}
	}

	checksumsCtx, checksumsCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer checksumsCancel()

	checksumsReq, err := http.NewRequestWithContext(checksumsCtx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return &UpdateInfo{HasUpdate: false}
	}
	checksumsResp, err := dlClient.Do(checksumsReq)
	if err != nil {
		return &UpdateInfo{HasUpdate: false}
	}
	defer checksumsResp.Body.Close()

	if checksumsResp.StatusCode != http.StatusOK {
		return &UpdateInfo{HasUpdate: false}
	}

	checksumsBytes, err := io.ReadAll(checksumsResp.Body)
	if err != nil {
		return &UpdateInfo{HasUpdate: false}
	}

	var expectedSHA string
	for _, line := range strings.Split(string(checksumsBytes), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == archiveName {
			expectedSHA = parts[0]
			break
		}
	}
	if expectedSHA == "" {
		return &UpdateInfo{HasUpdate: false}
	}

	if err := validateChecksum(tmpPath, expectedSHA); err != nil {
		return &UpdateInfo{HasUpdate: false}
	}

	if err := os.MkdirAll(stageDir, 0700); err != nil {
		return &UpdateInfo{HasUpdate: false}
	}

	if err := extractBinary(tmpPath, stageBin); err != nil {
		return &UpdateInfo{HasUpdate: false}
	}

	if err := os.Chmod(stageBin, 0755); err != nil {
		os.RemoveAll(stageDir)
		return &UpdateInfo{HasUpdate: false}
	}

	meta := fmt.Sprintf(`{"version":"%s"}`, latestVersion)
	if err := os.WriteFile(filepath.Join(stageDir, "metadata.json"), []byte(meta), 0600); err != nil {
		os.RemoveAll(stageDir)
		return &UpdateInfo{HasUpdate: false}
	}

	return &UpdateInfo{
		HasUpdate:      true,
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
	}
}

// ApplyStagedUpdate checks for a previously staged binary and applies it
// atomically to the current executable path. Returns true on success.
// Never crashes: returns false on any error.
func ApplyStagedUpdate(execPath string) bool {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	stageDir := filepath.Join(configDir, "aimux", ".staged-update")
	stageBin := filepath.Join(stageDir, "aimux")
	metaPath := filepath.Join(stageDir, "metadata.json")

	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return false
	}
	if _, err := os.Stat(stageBin); os.IsNotExist(err) {
		os.RemoveAll(stageDir)
		return false
	}

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		os.RemoveAll(stageDir)
		return false
	}

	log.Printf("Applying staged update: %s", strings.TrimSpace(string(metaBytes)))

	if err := atomicReplace(stageBin, execPath); err != nil {
		os.RemoveAll(stageDir)
		return false
	}

	os.RemoveAll(stageDir)
	return true
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
