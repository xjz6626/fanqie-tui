package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/xjz6626/fanqie-tui/internal/agent"
	"github.com/xjz6626/fanqie-tui/internal/fanqie"
)

func TestRenderSearchReply(t *testing.T) {
	reply := agent.Reply{
		Kind: agent.KindSearch, Title: "找到 1 本相关书籍", Text: "打开 1",
		Books: []fanqie.Book{{Title: "示例书", Author: "作者", Status: "连载", Score: 9.1}},
	}
	rendered := renderReply(reply, 60)
	for _, expected := range []string{"找到 1 本", "示例书", "作者", "打开 1"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered output missing %q: %s", expected, rendered)
		}
	}
}

func TestRenderBookIncludesOfficialScoreAndCounts(t *testing.T) {
	reply := agent.Reply{Kind: agent.KindBook, Book: &fanqie.Book{
		Title: "18号公寓", Author: "作者", Score: 8.7, ReadCount: 19_492,
		BookshelfCount: 123_456, WordCount: 2_073_771, ChapterCount: 976,
	}}
	rendered := renderReply(reply, 80)
	for _, expected := range []string{"8.7 分", "1.9万 阅读", "12.3万 加书架", "207.4万字", "976章"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered output missing %q: %s", expected, rendered)
		}
	}
}

func TestNewModelFocusesComposer(t *testing.T) {
	model := New(agent.New(nil), 0, "")
	if !model.input.Focused() {
		t.Fatal("composer should be focused at startup")
	}
}

func TestStartupNoticeIsOptionalAndVisible(t *testing.T) {
	base := New(agent.New(nil), time.Second, "")
	if got := len(base.messages); got != 1 {
		t.Fatalf("base messages=%d, want 1", got)
	}
	model := base.WithStartupNotice("已从默认配置自动登录")
	model.resize(80, 24)
	if got := len(model.messages); got != 2 || !strings.Contains(model.viewport.View(), "已从默认配置自动登录") {
		t.Fatalf("messages=%d view=%q", got, model.viewport.View())
	}
	if got := len(base.WithStartupNotice("  ").messages); got != 1 {
		t.Fatalf("blank notice messages=%d, want 1", got)
	}
}

func TestPaletteAdaptsToTerminalBackground(t *testing.T) {
	colors := map[string]lipgloss.AdaptiveColor{
		"accent":      accent,
		"accent soft": accentSoft,
		"text":        textColor,
		"muted":       mutedColor,
		"border":      borderColor,
		"error":       errorColor,
	}
	for name, color := range colors {
		if color.Light == "" || color.Dark == "" {
			t.Errorf("%s color must define both light and dark variants: %+v", name, color)
		}
		if color.Light == color.Dark {
			t.Errorf("%s color does not adapt to the terminal background: %+v", name, color)
		}
	}
}

func TestComposerUsesAdaptiveColors(t *testing.T) {
	model := New(agent.New(nil), 0, "")
	if got := model.input.FocusedStyle.Text.GetForeground(); got != textColor {
		t.Errorf("text color = %#v, want %#v", got, textColor)
	}
	if got := model.input.FocusedStyle.Placeholder.GetForeground(); got != mutedColor {
		t.Errorf("placeholder color = %#v, want %#v", got, mutedColor)
	}
	if got := model.input.FocusedStyle.Prompt.GetForeground(); got != accent {
		t.Errorf("prompt color = %#v, want %#v", got, accent)
	}
}

func TestSpinnerOnlyTicksWhileBusy(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	updated, command := model.Update(spinner.TickMsg{})
	if command != nil {
		t.Fatal("idle spinner should not schedule another tick")
	}
	if updated.(Model).busy {
		t.Fatal("spinner tick should not change idle state")
	}

	model.busy = true
	updated, command = model.Update(spinner.TickMsg{})
	if command == nil {
		t.Fatal("busy spinner should schedule its next tick")
	}
	if !updated.(Model).busy {
		t.Fatal("spinner tick should preserve busy state")
	}
}

func TestToolStatusRecognizesNaturalAndSlashCommands(t *testing.T) {
	tests := map[string]string{
		"搜索 三体":       "正在搜索公开书库",
		"/search 三体":  "正在搜索公开书库",
		"/more":       "正在搜索公开书库",
		"/popular":    "正在搜索公开书库",
		"编辑推荐":        "正在搜索公开书库",
		"出版榜":         "正在搜索公开书库",
		"/open 1":     "正在读取书籍信息",
		"/catalog":    "正在读取书籍信息",
		"/read 3":     "正在获取并解码章节",
		"/next":       "正在获取并解码章节",
		"/account":    "正在验证登录会话",
		"/login":      "正在验证并保存登录会话",
		"/logout":     "正在清除登录会话",
		"/bookshelf":  "正在读取账号数据",
		"已读章节":        "正在读取账号数据",
		"/categories": "正在读取分类榜",
		"分类 女 古风世情":   "正在读取分类榜",
		"/authors":    "正在读取作者作品",
		"作者 1":        "正在读取作者作品",
	}
	for input, want := range tests {
		if got := toolStatus(input); got != want {
			t.Errorf("toolStatus(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestEscapeShowsCancellationImmediately(t *testing.T) {
	cancelled := false
	model := New(agent.New(nil), time.Second, "")
	model.resize(80, 24)
	model.busy = true
	model.requestCancel = func() { cancelled = true }
	model.syncConversation(false)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if !cancelled || got.status != "正在取消…" {
		t.Fatalf("cancelled=%v status=%q", cancelled, got.status)
	}
	if !strings.Contains(got.viewport.View(), "正在取消…") {
		t.Fatal("cancellation status should be rendered immediately")
	}
}

func TestArrowKeysSwitchChaptersOnlyFromEmptyComposer(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(80, 24)
	model.bookTitle = "示例书"
	model.chapterIndex = 2
	model.chapterTotal = 10

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	got := updated.(Model)
	if !got.busy || command == nil {
		t.Fatal("right arrow should submit the next-chapter command while reading")
	}
	if last := got.messages[len(got.messages)-1].reply.Text; last != "/next" {
		t.Fatalf("submitted %q, want /next", last)
	}
	if got.requestCancel != nil {
		got.requestCancel()
	}

	model.input.SetValue("搜索 三体")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	got = updated.(Model)
	if got.busy || got.input.Value() != "搜索 三体" {
		t.Fatalf("left arrow should edit non-empty input, busy=%v value=%q", got.busy, got.input.Value())
	}
}

func TestArrowKeysDoNotSwitchChaptersWhileBusy(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(80, 24)
	model.chapterIndex = 2
	model.busy = true
	messageCount := len(model.messages)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	got := updated.(Model)
	if len(got.messages) != messageCount {
		t.Fatal("busy model should not submit a chapter command")
	}
}

func TestReadingStyleCyclesAndAcceptsFontCommand(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(80, 24)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF2})
	got := updated.(Model)
	if got.readingStyle != readingStyleBold {
		t.Fatalf("F2 style=%v, want bold", got.readingStyle)
	}

	got.input.SetValue("/font relaxed")
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)
	if got.readingStyle != readingStyleRelaxed || got.input.Value() != "" || got.busy {
		t.Fatalf("font command did not apply locally: style=%v value=%q busy=%v", got.readingStyle, got.input.Value(), got.busy)
	}
	if !strings.Contains(got.messages[len(got.messages)-1].reply.Text, "终端模拟器") {
		t.Fatal("font feedback should accurately describe the terminal limitation")
	}
}

func TestWithReadingStyleInitializesWithoutCallingPersistence(t *testing.T) {
	called := 0
	model := New(agent.New(nil), time.Second, "").WithReadingStyle("relaxed", func(string) error {
		called++
		return nil
	})
	if model.readingStyle != readingStyleRelaxed {
		t.Fatalf("initial style=%v, want relaxed", model.readingStyle)
	}
	if called != 0 {
		t.Fatalf("initialization called persistence %d times", called)
	}

	invalid := New(agent.New(nil), time.Second, "").WithReadingStyle("not-a-style", nil)
	if invalid.readingStyle != readingStyleStandard {
		t.Fatalf("unknown initial style=%v, want standard", invalid.readingStyle)
	}
}

func TestReadingStyleChangesInvokePersistenceWithCanonicalValue(t *testing.T) {
	var saved []string
	model := New(agent.New(nil), time.Second, "").WithReadingStyle("standard", func(style string) error {
		saved = append(saved, style)
		return nil
	})
	model.resize(80, 24)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF2})
	got := updated.(Model)
	got.input.SetValue("/font 宽松")
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)

	if want := []string{"bold", "relaxed"}; fmt.Sprint(saved) != fmt.Sprint(want) {
		t.Fatalf("saved styles=%v, want %v", saved, want)
	}
	if got.readingStyle != readingStyleRelaxed {
		t.Fatalf("final style=%v, want relaxed", got.readingStyle)
	}
}

func TestReadingStylePersistenceFailureIsNonFatalAndVisible(t *testing.T) {
	model := New(agent.New(nil), time.Second, "").WithReadingStyle("standard", func(string) error {
		return errors.New("磁盘只读")
	})
	model.resize(80, 24)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF2})
	got := updated.(Model)
	if got.readingStyle != readingStyleBold || got.busy {
		t.Fatalf("failed save should keep the in-session change: style=%v busy=%v", got.readingStyle, got.busy)
	}
	last := got.messages[len(got.messages)-1]
	if last.role != "error" || !strings.Contains(last.reply.Title, "未能保存") || !strings.Contains(got.viewport.View(), "磁盘只读") {
		t.Fatalf("persistence failure was not rendered: role=%q reply=%+v view=%q", last.role, last.reply, got.viewport.View())
	}
}

func TestRelaxedReadingStyleAddsLineSpacing(t *testing.T) {
	rendered := addReadingLineSpacing("　　第一行。\n　　第二行。")
	if rendered != "　　第一行。\n\n　　第二行。" {
		t.Fatalf("relaxed style did not add reading line spacing: %q", rendered)
	}
}

func TestFooterAdvertisesReaderShortcuts(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(120, 24)
	view := model.View()
	for _, hint := range []string{"F1 命令面板", "←/→ 章节", "F2 正文样式"} {
		if !strings.Contains(view, hint) {
			t.Fatalf("footer missing %q: %s", hint, view)
		}
	}
}

func TestCommandPanelTogglesAndFitsViewport(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(80, 24)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF1})
	got := updated.(Model)
	if !got.commandPanel || !strings.Contains(got.View(), "命令面板") || !strings.Contains(got.View(), "/sync") {
		t.Fatalf("command panel not visible: %s", got.View())
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(Model)
	if got.commandPanel {
		t.Fatal("escape should close the command panel")
	}

	got.busy = true
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyF1})
	if updated.(Model).commandPanel {
		t.Fatal("busy model should not open the command panel")
	}
}

func TestSlashCommandMenuFiltersNavigatesAndCompletes(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(80, 24)
	model.input.SetValue("/")
	if matches := model.slashCommandMatches(); len(matches) < 30 || matches[0].name != "/search" {
		t.Fatalf("initial matches=%+v", matches)
	}
	if view := model.View(); !strings.Contains(view, "命令补全") || !strings.Contains(view, "搜索公开书库") {
		t.Fatalf("slash menu not rendered: %s", view)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	if got.slashSelected != 1 {
		t.Fatalf("selected=%d, want 1", got.slashSelected)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyTab})
	got = updated.(Model)
	if got.input.Value() != "/open " || got.busy {
		t.Fatalf("tab completion value=%q busy=%v", got.input.Value(), got.busy)
	}

	got.input.SetValue("/sea")
	got.slashSelected = 0
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)
	if got.input.Value() != "/search " || got.busy {
		t.Fatalf("enter completion value=%q busy=%v", got.input.Value(), got.busy)
	}
}

func TestSlashCommandMenuEscapeDismissesUntilInputChanges(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(80, 24)
	model.input.SetValue("/")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if len(got.slashCommandMatches()) != 0 || strings.Contains(got.View(), "命令补全") {
		t.Fatal("escape should dismiss slash suggestions")
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got = updated.(Model)
	if len(got.slashCommandMatches()) == 0 || !strings.Contains(got.View(), "/search") {
		t.Fatalf("editing should reopen matching suggestions: %s", got.View())
	}
}

func TestExactSlashCommandStillSubmits(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(80, 24)
	model.input.SetValue("/status")
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if !got.busy || command == nil || got.input.Value() != "" {
		t.Fatalf("exact command should submit: busy=%v command=%v value=%q", got.busy, command != nil, got.input.Value())
	}
	if got.requestCancel != nil {
		got.requestCancel()
	}
}

func TestHeaderShowsSessionAndReadingProgress(t *testing.T) {
	provider := &tuiSessionProvider{}
	model := New(agent.New(provider), time.Second, "")
	model.resize(80, 24)
	model.bookTitle = "示例书"
	model.chapterIndex = 3
	model.chapterTotal = 10
	view := model.View()
	for _, expected := range []string{"番茄阅读", "官网同步", "示例书", "3/10", "▰"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("header missing %q: %s", expected, view)
		}
	}
}

type tuiSessionProvider struct{}

func (*tuiSessionProvider) Search(context.Context, string, int) (fanqie.SearchPage, error) {
	return fanqie.SearchPage{}, nil
}
func (*tuiSessionProvider) Discover(context.Context, fanqie.DiscoverKind, int) (fanqie.SearchPage, error) {
	return fanqie.SearchPage{}, nil
}
func (*tuiSessionProvider) GetBook(context.Context, string) (fanqie.Book, error) {
	return fanqie.Book{}, nil
}
func (*tuiSessionProvider) GetDirectory(context.Context, string) ([]fanqie.Chapter, error) {
	return nil, nil
}
func (*tuiSessionProvider) GetChapter(context.Context, string) (fanqie.ChapterContent, error) {
	return fanqie.ChapterContent{}, nil
}
func (*tuiSessionProvider) HasSession() bool  { return true }
func (*tuiSessionProvider) SetSession(string) {}
func (*tuiSessionProvider) ClearSession()     {}

func TestTurnErrorUsesFriendlyMessages(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		role  string
		title string
	}{
		{name: "cancelled", err: fmt.Errorf("wrapped: %w", context.Canceled), role: "assistant", title: "请求已取消"},
		{name: "timeout", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), role: "error", title: "请求超时"},
		{name: "other", err: errors.New("offline"), role: "error", title: "请求失败"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			role, title, text := turnError(test.err)
			if role != test.role || title != test.title || text == "" {
				t.Fatalf("got role=%q title=%q text=%q", role, title, text)
			}
		})
	}
}

func TestTerminalTextRemovesControlSequences(t *testing.T) {
	input := "正文\x1b]2;恶意标题\a保留\x1b[31m红色\x1b[0m\n下一行\x00"
	if got, want := terminalText(input), "正文保留红色\n下一行"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderReplySanitizesUpstreamText(t *testing.T) {
	reply := agent.Reply{Kind: agent.KindChapter, Chapter: &fanqie.ChapterContent{
		Title: "标题\x1b[2J", Content: "正文\x1b]2;changed\a保留",
	}}
	rendered := renderReply(reply, 60)
	if strings.Contains(rendered, "\x1b[2J") || strings.Contains(rendered, "changed") || !strings.Contains(rendered, "正文保留") {
		t.Fatalf("unsafe rendering: %q", rendered)
	}
}

func TestViewTruncatesLongContextToTerminalWidth(t *testing.T) {
	model := New(agent.New(nil), time.Second, "")
	model.resize(60, 20)
	model.bookTitle = strings.Repeat("很长的书名", 20)
	view := model.View()
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > model.width {
			t.Fatalf("line width %d exceeds terminal width %d: %q", lipgloss.Width(line), model.width, line)
		}
	}
}

func TestViewFitsNarrowAndShortTerminals(t *testing.T) {
	for _, size := range []struct{ width, height int }{{20, 24}, {23, 24}, {24, 24}, {30, 7}, {30, 8}, {32, 12}, {32, 14}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			model := New(agent.New(nil), time.Second, "")
			model.resize(size.width, size.height)
			if size.width >= 24 && size.height >= 8 {
				model.input.SetValue("/")
			}
			view := model.View()
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.width {
					t.Fatalf("line width %d exceeds terminal width %d: %q", lipgloss.Width(line), size.width, line)
				}
			}
			if lipgloss.Height(view) > size.height {
				t.Fatalf("view height %d exceeds terminal height %d", lipgloss.Height(view), size.height)
			}
		})
	}
}

func TestRenderSearchReplyKeepsEverySelectableResultVisible(t *testing.T) {
	books := make([]fanqie.Book, 8)
	for index := range books {
		books[index] = fanqie.Book{Title: fmt.Sprintf("书籍 %d", index+1)}
	}
	rendered := renderReply(agent.Reply{Kind: agent.KindSearch, Books: books}, 60)
	if !strings.Contains(rendered, "书籍 8") || strings.Contains(rendered, "另有") {
		t.Fatalf("unexpected rendering: %s", rendered)
	}
}

func TestRenderChapterDoesNotRepeatExistingOrder(t *testing.T) {
	reply := agent.Reply{Kind: agent.KindChapter, Chapter: &fanqie.ChapterContent{
		Title: "第1章 开篇", Order: 1, Content: "正文",
	}}
	rendered := renderReply(reply, 60)
	if strings.Contains(rendered, "第 1 章  第1章") || !strings.Contains(rendered, "输入“下一章”") {
		t.Fatalf("unexpected rendering: %s", rendered)
	}
}

func TestRenderChapterUsesChineseTypesetting(t *testing.T) {
	reply := agent.Reply{Kind: agent.KindChapter, Chapter: &fanqie.ChapterContent{
		Title: "开篇", Order: 1, Content: "第一段。\n\n第二段。",
	}}
	rendered := renderReply(reply, 20)
	for _, expected := range []string{"第1章　开篇", "　　第一段。", "　　第二段。"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered output missing %q: %s", expected, rendered)
		}
	}
}

func TestCompactNumber(t *testing.T) {
	for value, expected := range map[int]string{0: "0", 12_345: "1.2万", 100_000: "10万", 123_000_000: "1.2亿"} {
		if got := compactNumber(value); got != expected {
			t.Fatalf("compactNumber(%d)=%q, want %q", value, got, expected)
		}
	}
}
