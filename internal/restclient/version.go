package restclient

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// SGVersion is the StorageGRID software version.
type SGVersion struct {
	Major int
	Minor int
	// Full is the version as reported by the product-version API, eg 11.9.0
	Full string
}

// AtLeast returns true if the version is equal to or newer than major.minor.
func (v SGVersion) AtLeast(major int, minor int) bool {
	if v.Major != major {
		return v.Major > major
	}
	return v.Minor >= minor
}

// ParseSGVersion parses the productVersion reported by StorageGRID, eg 11.9.0 or 12.0.0-20250101.1234.abcdef
func ParseSGVersion(full string) (SGVersion, error) {
	parts := strings.SplitN(strings.TrimSpace(full), ".", 3)
	if len(parts) < 2 {
		return SGVersion{}, fmt.Errorf("unable to parse StorageGRID version %q", full)
	}
	major, err := strconv.Atoi(leadingDigits(parts[0]))
	if err != nil {
		return SGVersion{}, fmt.Errorf("unable to parse StorageGRID version %q", full)
	}
	minor, err := strconv.Atoi(leadingDigits(parts[1]))
	if err != nil {
		return SGVersion{}, fmt.Errorf("unable to parse StorageGRID version %q", full)
	}
	return SGVersion{Major: major, Minor: minor, Full: strings.TrimSpace(full)}, nil
}

func leadingDigits(s string) string {
	end := 0
	for end < len(s) && unicode.IsDigit(rune(s[end])) {
		end++
	}
	return s[:end]
}

// NormalizeAPIVersion accepts 4, v4, or V4 and returns v4.
func NormalizeAPIVersion(version string) string {
	version = strings.ToLower(strings.TrimSpace(version))
	if version == "" {
		return ""
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

// apiVersionFor returns the API version to use for a given software version.
// v4 was introduced with StorageGRID 11.8.
func apiVersionFor(version SGVersion) string {
	if version.AtLeast(11, 8) {
		return "v4"
	}
	return "v3"
}
