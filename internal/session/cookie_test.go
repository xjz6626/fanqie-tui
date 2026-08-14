package session

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLoadCookieFileHeaderFormats(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "plain", body: "sessionid=secret; theme=dark\n", want: "sessionid=secret; theme=dark"},
		{name: "header prefix", body: "Cookie: sessionid=secret; theme=dark\n", want: "sessionid=secret; theme=dark"},
		{name: "case insensitive prefix", body: "cOoKiE: sessionid=secret", want: "sessionid=secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := writePrivateFile(t, test.body)
			value, err := LoadCookieFile(path)
			if err != nil || value != test.want {
				t.Fatalf("value=%q err=%v", value, err)
			}
		})
	}
}

func TestParseNetscapeFiltersDomainAndExpiry(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	contents := strings.Join([]string{
		"# Netscape HTTP Cookie File",
		".fanqienovel.com\tTRUE\t/\tTRUE\t2000000100\tsessionid\tsecret",
		"fanqienovel.com\tFALSE\t/\tFALSE\t0\tsession_cookie\tkept",
		"#HttpOnly_.fanqienovel.com\tTRUE\t/\tTRUE\t2000000200\thttp_only\tyes",
		"api.fanqienovel.com\tFALSE\t/\tTRUE\t2000000200\tapi\tno",
		".fanqienovel.com\tTRUE\t/reader\tTRUE\t2000000200\tpath_scoped\tno",
		"evilfanqienovel.com\tFALSE\t/\tFALSE\t2000000200\tevil\tno",
		".example.com\tTRUE\t/\tFALSE\t2000000200\tother\tno",
		".fanqienovel.com\tTRUE\t/\tFALSE\t1999999999\texpired\tno",
	}, "\n")
	got, err := parseCookieFile([]byte(contents), now)
	if err != nil {
		t.Fatal(err)
	}
	want := "sessionid=secret; session_cookie=kept; http_only=yes"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseNetscapeRejectsNoUsableSession(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	for _, contents := range []string{
		"# Netscape HTTP Cookie File\n.example.com\tTRUE\t/\tFALSE\t0\ta\tb",
		"# Netscape HTTP Cookie File\n.fanqienovel.com\tTRUE\t/\tFALSE\t1\ta\tb",
	} {
		if _, err := parseCookieFile([]byte(contents), now); err == nil || !strings.Contains(err.Error(), "没有可用") {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestLoadCookieFileRejectsUnsafeInput(t *testing.T) {
	t.Run("permissions", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "session.cookie")
		if err := os.WriteFile(path, []byte("sessionid=secret"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadCookieFile(path); err == nil || !strings.Contains(err.Error(), "chmod 600") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("multiple raw lines", func(t *testing.T) {
		path := writePrivateFile(t, "one=1\ntwo=2")
		if _, err := LoadCookieFile(path); err == nil {
			t.Fatal("expected multiline input to fail")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "target.cookie")
		link := filepath.Join(directory, "link.cookie")
		if err := os.WriteFile(target, []byte("sessionid=secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadCookieFile(link); err == nil || !strings.Contains(err.Error(), "符号链接") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("fifo", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("non-blocking secure open is Linux-specific")
		}
		path := filepath.Join(t.TempDir(), "session.pipe")
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if _, err := LoadCookieFile(path); err == nil || !strings.Contains(err.Error(), "普通文件") {
			t.Fatalf("fifo err=%v", err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("FIFO check blocked for %v", elapsed)
		}
	})
	t.Run("oversized", func(t *testing.T) {
		path := writePrivateFile(t, strings.Repeat("x", maxCookieFileSize+1))
		if _, err := LoadCookieFile(path); err == nil || !strings.Contains(err.Error(), "64 KiB") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("nul", func(t *testing.T) {
		path := writePrivateFile(t, "session=a\x00b")
		if _, err := LoadCookieFile(path); err == nil || !strings.Contains(err.Error(), "无效字符") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestDefaultCookiePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix path expectations")
	}
	t.Run("absolute XDG", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/tmp/fanqie-config-test")
		got, err := DefaultCookiePath()
		want := "/tmp/fanqie-config-test/fanqie-tui/session.cookie"
		if err != nil || got != want {
			t.Fatalf("got=%q err=%v want=%q", got, err, want)
		}
	})
	t.Run("relative XDG ignored", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "relative")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		got, err := DefaultCookiePath()
		want := filepath.Join(home, ".config", "fanqie-tui", "session.cookie")
		if err != nil || got != want {
			t.Fatalf("got=%q err=%v want=%q", got, err, want)
		}
	})
}

func TestDefaultCookieLifecycle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	if _, path, err := LoadDefaultCookie(); !errors.Is(err, ErrNoSession) || path == "" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	path, err := SaveDefaultCookie("Cookie: sessionid=secret")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode=%04o", got)
	}
	directoryInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := directoryInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode=%04o", got)
	}
	cookie, loadedPath, err := LoadDefaultCookie()
	if err != nil || cookie != "sessionid=secret" || loadedPath != path {
		t.Fatalf("cookie=%q path=%q err=%v", cookie, loadedPath, err)
	}
	removedPath, err := RemoveDefaultCookie()
	if err != nil || removedPath != path {
		t.Fatalf("path=%q err=%v", removedPath, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat err=%v", err)
	}
	if _, err := RemoveDefaultCookie(); err != nil {
		t.Fatalf("idempotent remove: %v", err)
	}
}

func TestSaveDefaultCookieRejectsSymlinkedLeafDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, cookieDirectory)); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveDefaultCookie("sessionid=secret"); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("save through symlink err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(target, cookieFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secret unexpectedly written through symlink: %v", err)
	}
}

func TestSaveCookieFileReplacesAtomicallyAndSecurely(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "nested", "session.cookie")
	if err := SaveCookieFile(path, "old=secret"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveCookieFile(path, "new=secret"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new=secret\n" {
		t.Fatalf("contents=%q", contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode=%04o", got)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".session.cookie.tmp-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files=%v err=%v", matches, err)
	}
}

func TestSaveAndRemoveRejectSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "session.cookie")
	if err := os.WriteFile(target, []byte("untouched=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveCookieFile(link, "new=secret"); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("save err=%v", err)
	}
	if err := RemoveCookieFile(link); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("remove err=%v", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "untouched=1" {
		t.Fatalf("target=%q err=%v", contents, err)
	}
}

func writePrivateFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.cookie")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
