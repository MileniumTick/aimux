package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/mod/semver"
)

// githubRelease represents a GitHub release API response (partial).
type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckForUpdate checks GitHub Releases for a newer version.
// Never blocks startup: errors are swallowed, returns zero-value UpdateInfo
// with HasUpdate=false on any failure.
func CheckForUpdate(currentVersion string, httpClient *http.Client) UpdateInfo {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	release, err := fetchLatestRelease(httpClient, currentVersion)
	if err != nil {
		return UpdateInfo{CurrentVersion: currentVersion, HasUpdate: false}
	}

	latestVersion := release.TagName
	if len(latestVersion) > 0 && latestVersion[0] == 'v' {
		latestVersion = latestVersion[1:]
	}

	return compareVersions(currentVersion, latestVersion)
}

// compareVersions compares two versions using semver and returns UpdateInfo.
func compareVersions(current, latest string) UpdateInfo {
	info := UpdateInfo{
		CurrentVersion: current,
		LatestVersion:  latest,
	}
	if semver.Compare("v"+latest, "v"+current) > 0 {
		info.HasUpdate = true
	}
	return info
}

// fetchLatestRelease fetches the latest release from GitHub API.
func fetchLatestRelease(httpClient *http.Client, version string) (*githubRelease, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.github.com/repos/MileniumTick/aimux/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "aimux/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}
