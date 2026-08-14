// Package session imports and persists browser sessions without ever logging or
// returning account credentials other than to the caller that requested them.
package session

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxCookieFileSize = 64 << 10
	cookieDirectory   = "fanqie-tui"
	cookieFilename    = "session.cookie"
	officialDomain    = "fanqienovel.com"
)

// ErrNoSession means that no persisted default session is available.
var ErrNoSession = errors.New("没有已保存的登录会话")

// DefaultCookiePath returns the private session file used for automatic login.
// A relative XDG_CONFIG_HOME is intentionally ignored: XDG requires it to be
// absolute, and resolving it relative to an arbitrary working directory could
// place a secret in the project tree.
func DefaultCookiePath() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); root != "" && filepath.IsAbs(root) {
		return filepath.Join(root, cookieDirectory, cookieFilename), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("确定用户配置目录：%w", err)
	}
	if !filepath.IsAbs(home) {
		return "", errors.New("用户主目录不是绝对路径")
	}
	return filepath.Join(home, ".config", cookieDirectory, cookieFilename), nil
}

// LoadDefaultCookie loads the session used for automatic login. The returned
// path is populated even when the file does not exist so the UI can guide the
// user without guessing the configured location.
func LoadDefaultCookie() (cookie, path string, err error) {
	path, err = DefaultCookiePath()
	if err != nil {
		return "", "", err
	}
	cookie, err = LoadCookieFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", path, fmt.Errorf("%w", ErrNoSession)
	}
	return cookie, path, err
}

// LoadCookieFile reads either a one-line Cookie request-header value or a
// Netscape cookies.txt export. Netscape input is reduced to non-expired cookies
// whose domain and path allow them to be sent to fanqienovel.com itself.
func LoadCookieFile(path string) (string, error) {
	contents, err := readPrivateFile(path)
	if err != nil {
		return "", err
	}
	return parseCookieFile(contents, time.Now())
}

func readPrivateFile(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("Cookie 文件路径为空")
	}
	file, err := secureOpenCookie(path)
	if err != nil {
		return nil, fmt.Errorf("打开 Cookie 文件：%w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("读取 Cookie 文件信息：%w", err)
	}
	linkInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("复核 Cookie 文件路径：%w", err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Cookie 文件不能是符号链接")
	}
	if !os.SameFile(info, linkInfo) {
		return nil, errors.New("Cookie 文件在打开时发生变化")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("Cookie 文件必须是普通文件")
	}
	if info.Size() > maxCookieFileSize {
		return nil, errors.New("Cookie 文件超过 64 KiB")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("Cookie 文件权限过宽（当前 %04o），请执行 chmod 600", info.Mode().Perm())
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxCookieFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取 Cookie 文件：%w", err)
	}
	if len(contents) > maxCookieFileSize {
		return nil, errors.New("Cookie 文件超过 64 KiB")
	}
	return contents, nil
}

func parseCookieFile(contents []byte, now time.Time) (string, error) {
	if len(contents) == 0 {
		return "", errors.New("Cookie 文件为空")
	}
	if strings.IndexByte(string(contents), 0) >= 0 {
		return "", errors.New("Cookie 文件包含无效字符")
	}

	text := strings.TrimSpace(string(contents))
	if text == "" {
		return "", errors.New("Cookie 文件为空")
	}
	if !looksLikeNetscape(text) {
		return normalizeCookieHeader(text)
	}

	var pairs []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.SplitN(line, "\t", 7)
		if len(fields) != 7 {
			return "", fmt.Errorf("Netscape Cookie 第 %d 行格式无效", lineNumber)
		}
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fields[0])), ".")
		path := strings.TrimSpace(fields[2])
		if domain != officialDomain || path != "/" {
			continue
		}
		expires, err := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		if err != nil || expires < 0 {
			return "", fmt.Errorf("Netscape Cookie 第 %d 行过期时间无效", lineNumber)
		}
		if expires != 0 && expires <= now.Unix() {
			continue
		}
		name := strings.TrimSpace(fields[5])
		value := fields[6]
		if !validCookieName(name) || strings.ContainsAny(value, "\r\n\x00;") {
			return "", fmt.Errorf("Netscape Cookie 第 %d 行名称或值无效", lineNumber)
		}
		pairs = append(pairs, name+"="+value)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("解析 Netscape Cookie：%w", err)
	}
	if len(pairs) == 0 {
		return "", errors.New("Cookie 文件中没有可用的 fanqienovel.com 登录信息")
	}
	return strings.Join(pairs, "; "), nil
}

func looksLikeNetscape(text string) bool {
	if strings.Contains(text, "\n") || strings.Contains(text, "\r") {
		return true
	}
	return strings.Count(text, "\t") >= 6
}

func normalizeCookieHeader(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) >= len("cookie:") && strings.EqualFold(value[:len("cookie:")], "cookie:") {
		value = strings.TrimSpace(value[len("cookie:"):])
	}
	if value == "" {
		return "", errors.New("Cookie 文件为空")
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("Cookie 请求头必须只包含一行")
	}
	if !strings.Contains(value, "=") {
		return "", errors.New("Cookie 请求头格式无效")
	}
	return value, nil
}

func validCookieName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if character <= 0x20 || character >= 0x7f || strings.ContainsRune("()<>@,;:\"/[]?={}\\", character) {
			return false
		}
	}
	return true
}

// SaveDefaultCookie atomically saves a verified Cookie header for automatic
// login. Callers should verify the session against the service before saving.
func SaveDefaultCookie(cookie string) (string, error) {
	path, err := DefaultCookiePath()
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(path)
	if err := ensurePrivateDirectory(directory); err != nil {
		return path, err
	}
	if err := SaveCookieFile(path, cookie); err != nil {
		return path, err
	}
	return path, nil
}

func ensurePrivateDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("创建会话目录：%w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("检查会话目录：%w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("会话目录不能是符号链接")
	}
	if !info.IsDir() {
		return errors.New("会话目录必须是目录")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("设置会话目录权限：%w", err)
	}
	return nil
}

// SaveCookieFile validates and atomically writes a Cookie header with mode
// 0600. Temporary data is created beside the destination so rename is atomic.
func SaveCookieFile(path, cookie string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("Cookie 文件路径为空")
	}
	normalized, err := normalizeCookieHeader(cookie)
	if err != nil {
		return err
	}
	if len(normalized) > maxCookieFileSize {
		return errors.New("Cookie 文件超过 64 KiB")
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Cookie 文件不能是符号链接")
		}
		if !info.Mode().IsRegular() {
			return errors.New("Cookie 文件必须是普通文件")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("检查 Cookie 文件：%w", statErr)
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("创建会话目录：%w", err)
	}
	temporary, err := os.CreateTemp(directory, ".session.cookie.tmp-")
	if err != nil {
		return fmt.Errorf("创建临时 Cookie 文件：%w", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		_ = temporary.Close()
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("设置 Cookie 文件权限：%w", err)
	}
	if _, err := io.WriteString(temporary, normalized+"\n"); err != nil {
		return fmt.Errorf("写入 Cookie 文件：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("同步 Cookie 文件：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭 Cookie 文件：%w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("保存 Cookie 文件：%w", err)
	}
	keepTemporary = false
	return nil
}

// RemoveDefaultCookie logs out persistently by removing the automatic session.
// It is idempotent and returns the resolved path for user-facing confirmation.
func RemoveDefaultCookie() (string, error) {
	path, err := DefaultCookiePath()
	if err != nil {
		return "", err
	}
	return path, RemoveCookieFile(path)
}

// RemoveCookieFile removes a regular session file without following symlinks.
// A missing file already represents a logged-out state and is not an error.
func RemoveCookieFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("Cookie 文件路径为空")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查 Cookie 文件：%w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Cookie 文件不能是符号链接")
	}
	if !info.Mode().IsRegular() {
		return errors.New("Cookie 文件必须是普通文件")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除 Cookie 文件：%w", err)
	}
	return nil
}
