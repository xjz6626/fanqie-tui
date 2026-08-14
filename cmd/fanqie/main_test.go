package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xjz6626/fanqie-tui/internal/fanqie"
	"github.com/xjz6626/fanqie-tui/internal/session"
)

func TestResolveSessionVerifiesExplicitImportBeforeSaving(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	source := filepath.Join(t.TempDir(), "fanqienovel.com_cookies.txt")
	if err := os.WriteFile(source, []byte("Cookie: sessionid=test-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	verified := ""
	result, err := resolveSession(source, func(cookie string) error {
		verified = cookie
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if verified != "sessionid=test-value" || result.cookie != verified {
		t.Fatalf("explicit import was not verified and activated")
	}
	saved, path, err := session.LoadDefaultCookie()
	if err != nil || saved != verified {
		t.Fatalf("saved default session path=%q err=%v", path, err)
	}
	if !strings.Contains(result.notice, "通过") || !strings.Contains(result.description, "已验证") {
		t.Fatalf("unexpected login result: %+v", result)
	}
}

func TestResolveSessionDoesNotSaveRejectedImport(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	source := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(source, []byte("sessionid=expired\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveSession(source, func(string) error { return fanqie.ErrLoginRequired })
	if err == nil || !strings.Contains(err.Error(), "未保存") {
		t.Fatalf("rejected import error=%v", err)
	}
	if _, _, err := session.LoadDefaultCookie(); !errors.Is(err, session.ErrNoSession) {
		t.Fatalf("rejected import was persisted: %v", err)
	}
}

func TestResolveSessionAutomaticallyLoadsDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := session.SaveDefaultCookie("sessionid=saved"); err != nil {
		t.Fatal(err)
	}
	result, err := resolveSession("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.cookie != "sessionid=saved" || !strings.Contains(result.notice, "自动加载") {
		t.Fatalf("unexpected automatic login result: %+v", result)
	}
}

func TestResolveSessionGuidesDetectedCurrentDirectoryExport(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.WriteFile("fanqienovel.com_cookies.txt", []byte("sessionid=not-imported"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := resolveSession("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.cookie != "" || !strings.Contains(result.notice, "输入“登录”") || !strings.Contains(result.notice, "-cookie-file ./fanqienovel.com_cookies.txt") {
		t.Fatalf("unexpected discovery result: %+v", result)
	}
}

func TestResolveSessionDiscoversBareCooikeName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if err := os.WriteFile("cooike", []byte("sessionid=not-imported"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := resolveSession("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.cookie != "" || !strings.Contains(result.notice, "cooike") || !strings.Contains(result.guide, "./cooike") {
		t.Fatalf("unexpected bare cooike discovery: %+v", result)
	}
}

func TestResolveSessionUnsafeDefaultFallsBackToAnonymous(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := session.SaveDefaultCookie("sessionid=saved")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := resolveSession("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.cookie != "" || !strings.Contains(result.notice, "匿名模式") || !strings.Contains(result.notice, "chmod 600") {
		t.Fatalf("unsafe default did not fall back safely: %+v", result)
	}
}
