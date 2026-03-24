package ui

import (
	"strings"
)

// shortenHomePath replaces a leading /Users/<name>/ with ~/
// Used in group headers for both sessions and plans views.
func shortenHomePath(path string) string {
	if strings.HasPrefix(path, "/Users/") {
		parts := strings.SplitN(path, "/", 4)
		if len(parts) >= 3 {
			return "~/" + strings.Join(parts[3:], "/")
		}
	}
	return path
}
