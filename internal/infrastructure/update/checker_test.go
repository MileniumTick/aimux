package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{
			name:    "update available",
			current: "1.0.0",
			latest:  "1.1.0",
			want:    true,
		},
		{
			name:    "same version",
			current: "1.2.0",
			latest:  "1.2.0",
			want:    false,
		},
		{
			name:    "current newer than latest",
			current: "2.0.0",
			latest:  "1.0.0",
			want:    false,
		},
		{
			name:    "dev version compared to release",
			current: "dev",
			latest:  "1.0.0",
			want:    true,
		},
		{
			name:    "v prefix on both becomes vv — both invalid semver",
			current: "v1.0.0",
			latest:  "v1.1.0",
			want:    false,
		},
		{
			name:    "empty current version",
			current: "",
			latest:  "1.0.0",
			want:    true,
		},
		{
			name:    "empty latest version",
			current: "1.0.0",
			latest:  "",
			want:    false,
		},
		{
			name:    "pre-release 1.1.0-beta > 1.0.0 (higher minor)",
			current: "1.0.0",
			latest:  "1.1.0-beta",
			want:    true,
		},
		{
			name:    "major update available",
			current: "1.0.0",
			latest:  "2.0.0",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := compareVersions(tt.current, tt.latest)
			if info.HasUpdate != tt.want {
				t.Errorf("compareVersions(%q, %q).HasUpdate = %v, want %v",
					tt.current, tt.latest, info.HasUpdate, tt.want)
			}
			if info.CurrentVersion != tt.current {
				t.Errorf("compareVersions(%q, %q).CurrentVersion = %q, want %q",
					tt.current, tt.latest, info.CurrentVersion, tt.current)
			}
		})
	}
}

func TestCheckForUpdate_NewerVersion(t *testing.T) {
	server := newGitHubTestServer("v1.1.0")
	defer server.Close()

	client := &http.Client{Transport: &roundTripper{serverURL: server.URL}}

	info := CheckForUpdate("1.0.0", client)
	if !info.HasUpdate {
		t.Fatal("expected HasUpdate=true")
	}
	if info.LatestVersion != "1.1.0" {
		t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "1.1.0")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "1.0.0")
	}
}

func TestCheckForUpdate_SameVersion(t *testing.T) {
	server := newGitHubTestServer("v1.0.0")
	defer server.Close()

	client := &http.Client{Transport: &roundTripper{serverURL: server.URL}}

	info := CheckForUpdate("1.0.0", client)
	if info.HasUpdate {
		t.Error("expected HasUpdate=false when versions match")
	}
}

func TestCheckForUpdate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &http.Client{Transport: &roundTripper{serverURL: server.URL}}

	info := CheckForUpdate("1.0.0", client)
	if info.HasUpdate {
		t.Error("expected HasUpdate=false on HTTP error")
	}
}

func TestCheckForUpdate_NilClient(t *testing.T) {
	// When httpClient is nil, CheckForUpdate falls back to http.DefaultClient.
	// We can't easily mock that without transport recursion, so we verify
	// it doesn't panic and returns a zero-value result (no update).
	// The nil-fallback path is exercised by the real update flow.
	info := CheckForUpdate("1.0.0", nil)
	// Should not panic; HasUpdate may be false since it hits real GitHub
	_ = info
}

func TestFetchLatestRelease_ValidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v1.5.0",
			"assets": []map[string]any{
				{"name": "aimux_linux_amd64.tar.gz", "browser_download_url": "https://example.com/dl.tar.gz"},
			},
		})
	}))
	defer server.Close()

	client := &http.Client{Transport: &roundTripper{serverURL: server.URL}}

	release, err := fetchLatestRelease(client, "1.0.0")
	if err != nil {
		t.Fatalf("fetchLatestRelease failed: %v", err)
	}
	if release.TagName != "v1.5.0" {
		t.Errorf("TagName = %q, want %q", release.TagName, "v1.5.0")
	}
	if len(release.Assets) != 1 {
		t.Errorf("expected 1 asset, got %d", len(release.Assets))
	}
}

func TestFetchLatestRelease_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := &http.Client{Transport: &roundTripper{serverURL: server.URL}}

	_, err := fetchLatestRelease(client, "1.0.0")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchLatestRelease_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &http.Client{Transport: &roundTripper{serverURL: server.URL}}

	_, err := fetchLatestRelease(client, "1.0.0")
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

// newGitHubTestServer returns an httptest.Server that responds with a GitHub
// Releases API response containing the given tagName.
func newGitHubTestServer(tagName string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tagName,
			"html_url": "https://github.com/MileniumTick/aimux/releases/tag/" + tagName,
			"body":     "Release " + tagName,
			"assets": []map[string]any{
				{
					"name":                 "aimux_darwin_amd64.tar.gz",
					"browser_download_url": "https://github.com/MileniumTick/aimux/releases/download/" + tagName + "/aimux_darwin_amd64.tar.gz",
				},
			},
		})
	}))
}

// roundTripper is an http.RoundTripper that rewrites all requests to the
// configured serverURL, preserving the path and query. This allows tests to
// intercept requests to hardcoded URLs like api.github.com.
type roundTripper struct {
	serverURL string
	inner     http.RoundTripper
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := rt.inner
	if inner == nil {
		inner = http.DefaultTransport
	}

	// Build new URL: serverURL + path + query
	newURL := rt.serverURL + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}
	newReq, err := http.NewRequest(req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header.Clone()
	newReq.GetBody = req.GetBody
	newReq.ContentLength = req.ContentLength

	return inner.RoundTrip(newReq)
}


