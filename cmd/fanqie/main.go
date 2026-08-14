package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xjz6626/fanqie-tui/internal/agent"
	"github.com/xjz6626/fanqie-tui/internal/fanqie"
	"github.com/xjz6626/fanqie-tui/internal/library"
	"github.com/xjz6626/fanqie-tui/internal/session"
	"github.com/xjz6626/fanqie-tui/internal/tui"
)

var version = "0.7.1"

func main() {
	flags := flag.NewFlagSet("fanqie", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	timeout := flags.Duration("timeout", 30*time.Second, "网络请求超时")
	cookieFile := flags.String("cookie-file", "", "导入、验证并保存番茄浏览器 Cookie 文件")
	stateFile := flags.String("state-file", "", "本地历史、收藏与设置文件（默认遵循 XDG）")
	showVersion := flags.Bool("version", false, "显示版本")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "fanqie %s — 对话式终端小说阅读器\n\n", version)
		fmt.Fprintln(flags.Output(), "用法:")
		fmt.Fprintln(flags.Output(), "  fanqie [选项] [首条指令]")
		fmt.Fprintln(flags.Output(), "\n示例:")
		fmt.Fprintln(flags.Output(), "  fanqie")
		fmt.Fprintln(flags.Output(), "  fanqie \"搜索 三体\"")
		fmt.Fprintln(flags.Output(), "\n选项:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *showVersion {
		fmt.Printf("fanqie %s\n", version)
		return
	}
	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "fanqie: -timeout 必须大于 0")
		os.Exit(2)
	}

	login, err := resolveSession(*cookieFile, func(cookie string) error {
		candidate := fanqie.NewWebProviderWithSession(*timeout, cookie)
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		_, err := candidate.GetAccount(ctx)
		return err
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fanqie: %v\n", err)
		os.Exit(2)
	}
	provider := fanqie.NewWebProviderWithSession(*timeout, login.cookie)
	if login.cookie == "" {
		provider = fanqie.NewWebProvider(*timeout)
	}

	var localLibrary *library.Store
	if strings.TrimSpace(*stateFile) == "" {
		localLibrary, err = library.OpenDefault()
	} else {
		localLibrary, err = library.Open(*stateFile)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "fanqie: 无法打开本地资料库：%v\n", err)
		os.Exit(1)
	}

	readerAgent := agent.NewWithLibrary(provider, localLibrary).WithSessionActions(agent.SessionActions{
		LoginGuide:         login.guide,
		SessionDescription: login.description,
		Login: func(ctx context.Context) (string, error) {
			source := discoverCookieExport()
			if source == "" {
				return "", errors.New("当前目录没有发现 fanqienovel.com_cookies.txt、cookies.txt、cookie.txt、cooike.txt 或 cooike")
			}
			cookie, err := session.LoadCookieFile(source)
			if err != nil {
				return "", fmt.Errorf("无法导入 %s：%w", source, err)
			}
			candidate := fanqie.NewWebProviderWithSession(*timeout, cookie)
			if _, err := candidate.GetAccount(ctx); err != nil {
				if errors.Is(err, fanqie.ErrLoginRequired) {
					return "", errors.New("Cookie 未通过官网登录验证，可能已经过期；未保存")
				}
				return "", fmt.Errorf("官网验证失败，未保存：%w", err)
			}
			path, err := session.SaveDefaultCookie(cookie)
			if err != nil {
				return "", fmt.Errorf("登录已验证，但保存默认配置失败：%w", err)
			}
			provider.SetSession(cookie)
			return fmt.Sprintf("已验证当前目录的 %s，并保存到：\n%s\n\n账号功能已立即启用；以后直接启动 fanqie 即可自动登录。", source, path), nil
		},
		Logout: func(context.Context) error {
			provider.ClearSession()
			if _, err := session.RemoveDefaultCookie(); err != nil {
				return fmt.Errorf("当前进程已退出登录，但删除默认会话失败：%w", err)
			}
			return nil
		},
	})
	settings := localLibrary.Settings()
	readingStyle := tui.ReadingStyleStandard
	if settings.FontStyle == library.FontStyleBold {
		readingStyle = tui.ReadingStyleBold
	} else if settings.LineSpacing > 0 {
		readingStyle = tui.ReadingStyleRelaxed
	}
	model := tui.New(readerAgent, *timeout, strings.Join(flags.Args(), " ")).WithReadingStyle(readingStyle, func(style string) error {
		settings := localLibrary.Settings()
		switch style {
		case tui.ReadingStyleBold:
			settings.FontStyle = library.FontStyleBold
			settings.LineSpacing = 0
		case tui.ReadingStyleRelaxed:
			settings.FontStyle = library.FontStyleRegular
			settings.LineSpacing = 1
		default:
			settings.FontStyle = library.FontStyleRegular
			settings.LineSpacing = 0
		}
		return localLibrary.SetSettings(settings)
	}).WithStartupNotice(login.notice)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fanqie: %v\n", err)
		os.Exit(1)
	}
}

type loginResolution struct {
	cookie      string
	description string
	notice      string
	guide       string
}

// resolveSession keeps credential persistence outside of the website provider.
// Explicit imports are saved only after the supplied verifier succeeds. A
// normal startup only loads the private default file; an export in the current
// directory is detected for guidance, never silently imported.
func resolveSession(importPath string, verify func(string) error) (loginResolution, error) {
	defaultPath, err := session.DefaultCookiePath()
	if err != nil {
		return loginResolution{}, err
	}
	guide := loginGuide(defaultPath, discoverCookieExport())
	if strings.TrimSpace(importPath) != "" {
		cookie, err := session.LoadCookieFile(importPath)
		if err != nil {
			return loginResolution{}, fmt.Errorf("无法导入 Cookie：%w", err)
		}
		if verify == nil {
			return loginResolution{}, errors.New("无法验证 Cookie：验证器未配置")
		}
		if err := verify(cookie); err != nil {
			if errors.Is(err, fanqie.ErrLoginRequired) {
				return loginResolution{}, errors.New("Cookie 未通过官网登录验证，可能已经过期；未保存到默认配置")
			}
			return loginResolution{}, fmt.Errorf("验证 Cookie 失败，未保存到默认配置：%w", err)
		}
		savedPath, err := session.SaveDefaultCookie(cookie)
		if err != nil {
			return loginResolution{}, fmt.Errorf("Cookie 已通过验证，但无法保存：%w", err)
		}
		return loginResolution{
			cookie:      cookie,
			description: "本次导入已验证，并保存到默认配置",
			notice:      fmt.Sprintf("Cookie 已通过官网登录验证并安全保存。\n以后直接运行 fanqie 即可自动登录。\n保存位置：%s", savedPath),
			guide:       guide,
		}, nil
	}

	cookie, path, err := session.LoadDefaultCookie()
	if err == nil {
		return loginResolution{
			cookie:      cookie,
			description: "已从默认配置自动加载",
			notice:      fmt.Sprintf("已从默认配置自动加载登录会话。输入 /account 验证账号，输入 /cloud-history 查看云端进度。\n配置位置：%s", path),
			guide:       guide,
		}, nil
	}
	if !errors.Is(err, session.ErrNoSession) {
		return loginResolution{
			guide:  guide,
			notice: fmt.Sprintf("默认登录会话未能安全加载，将以匿名模式继续。\n输入 /login 查看修复与重新导入步骤。\n位置：%s\n原因：%v", path, err),
		}, nil
	}

	resolution := loginResolution{guide: guide}
	if candidate := discoverCookieExport(); candidate != "" {
		resolution.notice = fmt.Sprintf("检测到浏览器 Cookie 文件：%s\n直接输入“登录”即可验证并保存；也可退出后运行：fanqie -cookie-file %s", candidate, shellDisplayPath(candidate))
	}
	return resolution, nil
}

func discoverCookieExport() string {
	for _, name := range []string{"fanqienovel.com_cookies.txt", "cookies.txt", "cookie.txt", "cooike.txt", "cooike"} {
		info, err := os.Lstat(name)
		if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return name
		}
	}
	return ""
}

func shellDisplayPath(path string) string {
	if filepath.IsAbs(path) || strings.HasPrefix(path, ".") {
		return path
	}
	return "./" + path
}

func loginGuide(defaultPath, candidate string) string {
	source := "./fanqienovel.com_cookies.txt"
	if candidate != "" {
		source = shellDisplayPath(candidate)
	}
	return fmt.Sprintf("本程序不接收账号密码，请通过浏览器 Cookie 安全登录：\n\n  1. 在浏览器登录 fanqienovel.com。\n  2. 导出 Netscape cookies.txt，或保存单行 Cookie 请求头。\n  3. 执行 chmod 600 %s。\n  4. 执行 fanqie -cookie-file %s。\n\n程序只会向 https://fanqienovel.com 发送 Cookie；通过账号接口验证后才保存，以后直接运行 fanqie 即可自动登录。\n默认位置：%s\n输入 /account 验证、/cloud-history 查看云进度、/logout 清除默认会话。请勿粘贴、截图或提交 Cookie。", source, source, defaultPath)
}
