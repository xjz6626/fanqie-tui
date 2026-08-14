//go:build !linux

package session

import "os"

// Other platforms retain the same post-open identity and regular-file checks.
// Platform-specific no-follow implementations can replace this fallback.
func secureOpenCookie(path string) (*os.File, error) {
	return os.Open(path)
}
