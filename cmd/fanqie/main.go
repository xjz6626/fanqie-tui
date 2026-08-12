package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xjz6626/fanqie-tui/internal/agent"
	"github.com/xjz6626/fanqie-tui/internal/fanqie"
	"github.com/xjz6626/fanqie-tui/internal/tui"
)

var version = "0.2.0"

func main() {
	flags := flag.NewFlagSet("fanqie", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	timeout := flags.Duration("timeout", 30*time.Second, "网络请求超时")
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

	provider := fanqie.NewWebProvider(*timeout)
	readerAgent := agent.New(provider)
	model := tui.New(readerAgent, *timeout, strings.Join(flags.Args(), " "))
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fanqie: %v\n", err)
		os.Exit(1)
	}
}
