package update

// UpdateInfo carries the result of a version check.
type UpdateInfo struct {
	CurrentVersion string // e.g. "1.2.0" (no v prefix)
	LatestVersion  string // e.g. "1.3.0" (no v prefix)
	HasUpdate      bool
}
