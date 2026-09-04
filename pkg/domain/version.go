package domain

import "strings"

var (
	// Version is the canonical version of the Flagura platform.
	// Can be overridden at build time via -ldflags:
	// -X 'github.com/dhawalhost/flagura/pkg/domain.Version=v1.6.0'
	Version = "v1.6.0"
)

// CleanVersion returns the semantic version without any leading 'v' prefix (e.g. "1.6.0").
func CleanVersion() string {
	return strings.TrimPrefix(Version, "v")
}
