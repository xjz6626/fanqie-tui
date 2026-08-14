//go:build linux

package session

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// secureOpenCookie refuses symlinks in the kernel and opens special files
// non-blocking so a FIFO/device cannot stall before its type is checked.
func secureOpenCookie(path string) (*os.File, error) {
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, errors.New("Cookie 文件不能是符号链接")
		}
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), path), nil
}
