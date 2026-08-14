// Package tui presents the conversational reader in a full-screen terminal UI.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/xjz6626/fanqie-tui/internal/agent"
	"github.com/xjz6626/fanqie-tui/internal/fanqie"
)

var (
	accent      = lipgloss.AdaptiveColor{Light: "#B93815", Dark: "#F36D4A"}
	accentSoft  = lipgloss.AdaptiveColor{Light: "#8F351D", Dark: "#FFB39D"}
	textColor   = lipgloss.AdaptiveColor{Light: "#262220", Dark: "#E8E8E8"}
	mutedColor  = lipgloss.AdaptiveColor{Light: "#68615D", Dark: "#96908D"}
	borderColor = lipgloss.AdaptiveColor{Light: "#B8AEA8", Dark: "#514946"}
	errorColor  = lipgloss.AdaptiveColor{Light: "#B42318", Dark: "#FF6B6B"}
)

type message struct {
	role  string
	reply agent.Reply
}

type turnResult struct {
	replies []agent.Reply
	err     error
	book    string
	index   int
	total   int
}

type slashCommand struct {
	name        string
	arguments   string
	description string
}

var slashCommands = []slashCommand{
	{name: "/search", arguments: "<书名或作者>", description: "搜索公开书库"},
	{name: "/open", arguments: "<序号>", description: "打开搜索结果"},
	{name: "/read", arguments: "<章节号>", description: "跳转到指定章节"},
	{name: "/next", description: "阅读下一章"},
	{name: "/prev", description: "返回上一章"},
	{name: "/catalog", description: "查看当前目录"},
	{name: "/popular", description: "查看热门榜"},
	{name: "/recommend", description: "查看编辑推荐"},
	{name: "/male", description: "查看男频精选"},
	{name: "/female", description: "查看女频精选"},
	{name: "/updates", description: "查看最近更新"},
	{name: "/published", description: "查看出版榜"},
	{name: "/more", description: "加载更多结果"},
	{name: "/categories", description: "列出小说分类"},
	{name: "/category", arguments: "<male|female> <分类>", description: "查看分类榜"},
	{name: "/authors", description: "查看热门作者"},
	{name: "/author", arguments: "<序号或作者 ID>", description: "查看作者与作品"},
	{name: "/favorite", description: "加入官网书架"},
	{name: "/unfavorite", description: "移出官网书架"},
	{name: "/favorites", description: "查看收藏或官网书架"},
	{name: "/bookshelf", description: "刷新官网书架"},
	{name: "/sync", description: "同步账号、书架与进度"},
	{name: "/history", description: "查看本地阅读历史"},
	{name: "/resume", description: "恢复上次阅读"},
	{name: "/cloud-history", description: "查看官网阅读进度"},
	{name: "/read-items", description: "查看当前书已读章节"},
	{name: "/reviews", description: "查看当前书公开书评"},
	{name: "/review-feed", description: "浏览官网最新书评"},
	{name: "/comment", arguments: "<书评序号|链接|ID>", description: "打开书评与回复"},
	{name: "/account", description: "验证登录状态"},
	{name: "/login", description: "导入并保存浏览器登录"},
	{name: "/logout", description: "清除本机登录会话"},
	{name: "/font", arguments: "<standard|bold|relaxed>", description: "切换正文显示模式"},
	{name: "/status", description: "查看当前阅读状态"},
	{name: "/help", description: "显示完整使用帮助"},
	{name: "/clear", description: "清空当前消息流"},
	{name: "/quit", description: "退出程序"},
}

type readingStyle uint8

// Canonical values accepted and emitted by WithReadingStyle.
const (
	ReadingStyleStandard = "standard"
	ReadingStyleBold     = "bold"
	ReadingStyleRelaxed  = "relaxed"
)

const (
	readingStyleStandard readingStyle = iota
	readingStyleBold
	readingStyleRelaxed
)

func (style readingStyle) label() string {
	switch style {
	case readingStyleBold:
		return "加粗"
	case readingStyleRelaxed:
		return "宽松"
	default:
		return "标准"
	}
}

func (style readingStyle) next() readingStyle {
	return (style + 1) % 3
}

// Model is the Bubble Tea application model.
type Model struct {
	agent          *agent.State
	input          textarea.Model
	viewport       viewport.Model
	spinner        spinner.Model
	messages       []message
	width          int
	height         int
	ready          bool
	busy           bool
	status         string
	requestCancel  context.CancelFunc
	timeout        time.Duration
	initialPrompt  string
	bookTitle      string
	chapterIndex   int
	chapterTotal   int
	readingStyle   readingStyle
	onStyleChange  func(string) error
	commandPanel   bool
	slashSelected  int
	slashDismissed string
}

// New creates a conversation UI for a local reader agent.
func New(readerAgent *agent.State, timeout time.Duration, initialPrompt string) Model {
	input := textarea.New()
	input.Placeholder = "输入书名，或试试“热门榜”…"
	input.Prompt = "› "
	input.CharLimit = 500
	input.MaxHeight = 5
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(accent).Bold(true)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(textColor)
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(mutedColor)
	input.BlurredStyle = input.FocusedStyle
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+j"))
	_ = input.Focus()

	loading := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	loading.Style = lipgloss.NewStyle().Foreground(accent)

	return Model{
		agent:         readerAgent,
		input:         input,
		spinner:       loading,
		status:        "就绪",
		timeout:       timeout,
		initialPrompt: strings.TrimSpace(initialPrompt),
		messages: []message{{role: "assistant", reply: agent.Reply{
			Kind:  agent.KindText,
			Title: "阅读工作台",
			Text:  "搜索、榜单、官网书架、书评和阅读进度都集中在这里。\n\n发现：热门榜 · 分类 · 热门作者 · 最新书评\n阅读：搜索 三体 · 历史记录 · 继续阅读\n账号：登录 · 同步账号 · 官网书架\n\n按 F1 或 Ctrl+K 打开快捷面板。",
		}}},
	}
}

// WithReadingStyle configures the initial chapter display style and an
// optional persistence callback. Supported styles are standard, bold and
// relaxed. Unknown initial values fall back to standard. The callback runs
// only after a successful user-initiated change, not during initialization.
func (model Model) WithReadingStyle(initial string, onChange func(string) error) Model {
	if style, ok := parseReadingStyle(initial); ok {
		model.readingStyle = style
	} else {
		model.readingStyle = readingStyleStandard
	}
	model.onStyleChange = onChange
	model.syncConversation(false)
	return model
}

// WithStartupNotice appends a non-secret login/session notice to the opening
// conversation. It is intended for messages such as “已自动登录” or
// “发现 Cookie，等待导入”; callers must not include credential contents.
func (model Model) WithStartupNotice(notice string) Model {
	notice = strings.TrimSpace(notice)
	if notice == "" {
		return model
	}
	model.messages = append(model.messages, message{role: "assistant", reply: agent.Reply{
		Kind:  agent.KindStatus,
		Title: "登录状态",
		Text:  notice,
	}})
	model.syncConversation(false)
	return model
}

// Init focuses the composer and optionally dispatches an initial instruction.
func (model Model) Init() tea.Cmd {
	commands := []tea.Cmd{textarea.Blink}
	if model.initialPrompt != "" {
		commands = append(commands, func() tea.Msg { return initialPromptMsg(model.initialPrompt) })
	}
	return tea.Batch(commands...)
}

type initialPromptMsg string

// Update processes terminal and completed tool messages.
func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		model.resize(typed.Width, typed.Height)
		return model, nil
	case initialPromptMsg:
		if model.applyReadingStyleCommand(string(typed)) {
			return model, nil
		}
		return model.submit(string(typed))
	case turnResult:
		model.busy = false
		model.requestCancel = nil
		model.status = "就绪"
		model.bookTitle = typed.book
		model.chapterIndex = typed.index
		model.chapterTotal = typed.total
		model.updatePlaceholder()
		if typed.err != nil {
			role, title, text := turnError(typed.err)
			model.messages = append(model.messages, message{role: role, reply: agent.Reply{
				Kind: agent.KindText, Title: title, Text: text,
			}})
		} else {
			for _, reply := range typed.replies {
				switch reply.Kind {
				case agent.KindQuit:
					return model, tea.Quit
				case agent.KindClear:
					model.messages = nil
				default:
					if reply.Kind == agent.KindChapter {
						model.foldPreviousChapters()
					}
					model.messages = append(model.messages, message{role: "assistant", reply: reply})
				}
			}
		}
		model.syncConversation(true)
		return model, model.input.Focus()
	case spinner.TickMsg:
		if !model.busy {
			return model, nil
		}
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(typed)
		model.syncConversation(false)
		return model, command
	case tea.KeyMsg:
		switch typed.String() {
		case "f1", "ctrl+k":
			if !model.busy {
				model.commandPanel = !model.commandPanel
			}
			return model, nil
		case "ctrl+c":
			if model.busy && model.requestCancel != nil {
				if model.status == "正在取消…" {
					return model, tea.Quit
				}
				model.requestCancel()
				model.status = "正在取消…"
				model.syncConversation(false)
				return model, nil
			}
			return model, tea.Quit
		case "esc":
			if model.commandPanel {
				model.commandPanel = false
				return model, nil
			}
			if len(model.slashCommandMatches()) > 0 {
				model.slashDismissed = model.input.Value()
				return model, nil
			}
			if model.busy && model.requestCancel != nil {
				model.requestCancel()
				model.status = "正在取消…"
				model.syncConversation(false)
			}
			return model, nil
		case "enter":
			if !model.busy && strings.TrimSpace(model.input.Value()) != "" {
				if model.shouldCompleteSlashCommand() {
					model.completeSlashCommand()
					return model, nil
				}
				if handled := model.applyReadingStyleCommand(model.input.Value()); handled {
					return model, nil
				}
				return model.submit(model.input.Value())
			}
			return model, nil
		case "up", "down":
			if matches := model.slashCommandMatches(); len(matches) > 0 {
				if typed.String() == "up" {
					model.slashSelected = (model.normalizedSlashSelection(len(matches)) - 1 + len(matches)) % len(matches)
				} else {
					model.slashSelected = (model.normalizedSlashSelection(len(matches)) + 1) % len(matches)
				}
				return model, nil
			}
		case "tab":
			if len(model.slashCommandMatches()) > 0 && model.canCompleteSlashCommand() {
				model.completeSlashCommand()
				return model, nil
			}
		case "f2":
			if !model.busy {
				model.setReadingStyle(model.readingStyle.next(), true, true)
			}
			return model, nil
		case "left", "right":
			if !model.busy && model.chapterIndex > 0 && strings.TrimSpace(model.input.Value()) == "" {
				instruction := "/prev"
				if typed.String() == "right" {
					instruction = "/next"
				}
				return model.submit(instruction)
			}
		case "pgup", "pgdown":
			var command tea.Cmd
			model.viewport, command = model.viewport.Update(typed)
			return model, command
		}
	}

	var commands []tea.Cmd
	if !model.busy {
		previousInput := model.input.Value()
		var inputCommand tea.Cmd
		model.input, inputCommand = model.input.Update(msg)
		commands = append(commands, inputCommand)
		if model.input.Value() != previousInput {
			model.slashSelected = 0
			model.slashDismissed = ""
		}
	}
	var viewportCommand tea.Cmd
	model.viewport, viewportCommand = model.viewport.Update(msg)
	commands = append(commands, viewportCommand)
	return model, tea.Batch(commands...)
}

func (model *Model) applyReadingStyleCommand(value string) bool {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(fields) == 0 || (fields[0] != "/font" && fields[0] != "/字体") {
		return false
	}

	style := model.readingStyle.next()
	valid := true
	if len(fields) > 1 {
		style, valid = parseReadingStyle(fields[1])
	}

	model.input.Reset()
	if !valid || len(fields) > 2 {
		model.messages = append(model.messages, message{role: "assistant", reply: agent.Reply{
			Kind:  agent.KindText,
			Title: "正文显示模式",
			Text:  "用法：/font standard、/font bold 或 /font relaxed（中文可用：标准、加粗、宽松）。终端字体家族仍由终端设置控制。",
		}})
		model.syncConversation(true)
		return true
	}

	model.setReadingStyle(style, true, true)
	return true
}

func parseReadingStyle(value string) (readingStyle, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ReadingStyleStandard, "normal", "标准", "默认":
		return readingStyleStandard, true
	case ReadingStyleBold, "加粗":
		return readingStyleBold, true
	case ReadingStyleRelaxed, "宽松":
		return readingStyleRelaxed, true
	default:
		return readingStyleStandard, false
	}
}

func (model *Model) setReadingStyle(style readingStyle, announce, persist bool) {
	model.readingStyle = style
	if announce {
		model.messages = append(model.messages, message{role: "assistant", reply: agent.Reply{
			Kind:  agent.KindText,
			Title: "正文显示：" + style.label(),
			Text:  "已调整应用内的正文粗细或行距；字体家族仍由你的终端模拟器控制。",
		}})
	}
	if persist && model.onStyleChange != nil {
		if err := model.onStyleChange(style.String()); err != nil {
			model.messages = append(model.messages, message{role: "error", reply: agent.Reply{
				Kind:  agent.KindText,
				Title: "显示模式未能保存",
				Text:  fmt.Sprintf("本次会话仍使用“%s”模式；下次启动可能恢复原设置。保存错误：%v", style.label(), err),
			}})
		}
	}
	model.syncConversation(announce)
}

func (style readingStyle) String() string {
	switch style {
	case readingStyleBold:
		return ReadingStyleBold
	case readingStyleRelaxed:
		return ReadingStyleRelaxed
	default:
		return ReadingStyleStandard
	}
}

func turnError(err error) (role, title, text string) {
	switch {
	case errors.Is(err, context.Canceled):
		return "assistant", "请求已取消", "可以继续输入其他指令。"
	case errors.Is(err, context.DeadlineExceeded):
		return "error", "请求超时", "上游响应时间过长，可以稍后重试或通过 -timeout 调大超时时间。"
	default:
		return "error", "请求失败", err.Error()
	}
}

func (model *Model) foldPreviousChapters() {
	for index := range model.messages {
		chapter := model.messages[index].reply.Chapter
		if model.messages[index].reply.Kind == agent.KindChapter && chapter != nil && chapter.Content != "" {
			chapter.Content = "（上一章正文已折叠，可输入对应章节号重新打开。）"
		}
	}
}

func (model Model) submit(value string) (tea.Model, tea.Cmd) {
	instruction := strings.TrimSpace(value)
	if instruction == "" || model.busy {
		return model, nil
	}
	model.input.Reset()
	model.input.Blur()
	model.commandPanel = false
	model.slashSelected = 0
	model.slashDismissed = ""
	model.messages = append(model.messages, message{role: "user", reply: agent.Reply{Kind: agent.KindText, Text: instruction}})
	model.busy = true
	model.status = toolStatus(instruction)
	model.syncConversation(true)

	ctx, cancel := context.WithTimeout(context.Background(), model.timeout)
	model.requestCancel = cancel
	command := func() tea.Msg {
		defer cancel()
		replies, err := model.agent.Handle(ctx, instruction)
		book, index, total := model.agent.BookContext()
		return turnResult{replies: replies, err: err, book: book, index: index, total: total}
	}
	return model, tea.Batch(command, model.spinner.Tick)
}

func (model *Model) updatePlaceholder() {
	if model.bookTitle == "" {
		model.input.Placeholder = "搜索书名，或输入“热门榜”…"
		return
	}
	model.input.Placeholder = "输入章节号、书评、收藏，或用 ←/→ 切章…"
}

func (model Model) slashCommandMatches() []slashCommand {
	if model.busy || model.commandPanel {
		return nil
	}
	value := model.input.Value()
	if value == "" || value == model.slashDismissed || strings.ContainsAny(value, "\r\n") || !strings.HasPrefix(value, "/") {
		return nil
	}
	fields := strings.Fields(value)
	prefix := "/"
	if len(fields) > 0 {
		prefix = strings.ToLower(fields[0])
	}
	matches := make([]slashCommand, 0, len(slashCommands))
	for _, command := range slashCommands {
		if strings.HasPrefix(command.name, prefix) {
			matches = append(matches, command)
		}
	}
	return matches
}

func (model Model) normalizedSlashSelection(matchCount int) int {
	if matchCount <= 0 || model.slashSelected < 0 || model.slashSelected >= matchCount {
		return 0
	}
	return model.slashSelected
}

func (model Model) selectedSlashCommand() (slashCommand, bool) {
	matches := model.slashCommandMatches()
	if len(matches) == 0 {
		return slashCommand{}, false
	}
	return matches[model.normalizedSlashSelection(len(matches))], true
}

func (model Model) canCompleteSlashCommand() bool {
	command, ok := model.selectedSlashCommand()
	if !ok {
		return false
	}
	value := model.input.Value()
	fields := strings.Fields(value)
	if len(fields) == 0 || strings.ToLower(fields[0]) != command.name {
		return true
	}
	return strings.TrimSpace(value) == fields[0]
}

func (model Model) shouldCompleteSlashCommand() bool {
	command, ok := model.selectedSlashCommand()
	if !ok || !model.canCompleteSlashCommand() {
		return false
	}
	fields := strings.Fields(model.input.Value())
	if len(fields) == 0 || strings.ToLower(fields[0]) != command.name {
		return true
	}
	return command.arguments != "" && model.input.Value() == fields[0]
}

func (model *Model) completeSlashCommand() {
	command, ok := model.selectedSlashCommand()
	if !ok || !model.canCompleteSlashCommand() {
		return
	}
	value := command.name
	if command.arguments != "" {
		value += " "
	}
	model.input.SetValue(value)
	model.slashSelected = 0
	model.slashDismissed = ""
}

func toolStatus(instruction string) string {
	normalized := strings.ToLower(strings.TrimSpace(instruction))
	switch {
	case normalized == "/categories", strings.HasPrefix(normalized, "/category "), strings.HasPrefix(normalized, "分类"):
		return "正在读取分类榜"
	case normalized == "/authors", strings.HasPrefix(normalized, "/author "), strings.Contains(normalized, "作者"), strings.Contains(normalized, "作家"):
		return "正在读取作者作品"
	case strings.Contains(normalized, "搜索"), strings.Contains(normalized, "搜"),
		strings.Contains(normalized, "更多"), normalized == "下一页",
		strings.Contains(normalized, "榜"), strings.Contains(normalized, "推荐"),
		strings.HasPrefix(normalized, "/search"), normalized == "/more",
		normalized == "/popular", normalized == "/recommend", normalized == "/male",
		normalized == "/female", normalized == "/updates", normalized == "/recent",
		normalized == "/published":
		return "正在搜索公开书库"
	case strings.Contains(normalized, "目录"), strings.Contains(normalized, "打开"),
		strings.HasPrefix(normalized, "/open"), normalized == "/catalog":
		return "正在读取书籍信息"
	case normalized == "/logout", strings.Contains(normalized, "退出登录"), strings.Contains(normalized, "注销登录"):
		return "正在清除登录会话"
	case normalized == "/login", normalized == "登录":
		return "正在验证并保存登录会话"
	case normalized == "/favorite", normalized == "/unfavorite", strings.Contains(normalized, "收藏"), strings.Contains(normalized, "加入书架"), strings.Contains(normalized, "移出书架"):
		return "正在同步官网书架"
	case normalized == "/reviews", normalized == "/review-feed", strings.HasPrefix(normalized, "/comment "),
		strings.Contains(normalized, "书评"), strings.Contains(normalized, "评论"):
		return "正在读取官网书评"
	case normalized == "/bookshelf", normalized == "/cloud-history", normalized == "/read-items",
		normalized == "/sync", normalized == "同步", strings.Contains(normalized, "同步账号"),
		strings.Contains(normalized, "云端历史"), strings.Contains(normalized, "云端进度"),
		strings.Contains(normalized, "官网书架"), strings.Contains(normalized, "已读"):
		return "正在读取账号数据"
	case strings.Contains(normalized, "章"), strings.Contains(normalized, "继续"),
		strings.HasPrefix(normalized, "/read"), normalized == "/next",
		normalized == "/prev", normalized == "/previous":
		return "正在获取并解码章节"
	case normalized == "/account", strings.Contains(normalized, "登录状态"):
		return "正在验证登录会话"
	default:
		return "正在处理"
	}
}

func (model *Model) resize(width, height int) {
	model.width = width
	model.height = height
	contentWidth := max(20, width-4)
	model.input.SetWidth(max(10, contentWidth-4))
	inputHeight := max(3, model.input.Height()+2)
	viewportHeight := max(3, height-inputHeight-6)
	if !model.ready {
		model.viewport = viewport.New(contentWidth, viewportHeight)
		model.viewport.MouseWheelEnabled = true
		model.viewport.MouseWheelDelta = 3
		model.ready = true
	} else {
		model.viewport.Width = contentWidth
		model.viewport.Height = viewportHeight
	}
	model.syncConversation(false)
}

func (model *Model) syncConversation(bottom bool) {
	if !model.ready {
		return
	}
	blocks := make([]string, 0, len(model.messages)+1)
	for _, item := range model.messages {
		blocks = append(blocks, model.renderMessage(item))
	}
	if model.busy {
		blocks = append(blocks, lipgloss.NewStyle().Foreground(mutedColor).Render(model.spinner.View()+" "+model.status))
	}
	model.viewport.SetContent(strings.Join(blocks, "\n\n"))
	if bottom {
		latestHeight := 0
		if len(blocks) > 0 {
			latestHeight = lipgloss.Height(blocks[len(blocks)-1])
		}
		startOfLatest := model.viewport.TotalLineCount() - latestHeight - 1
		maxOffset := max(0, model.viewport.TotalLineCount()-model.viewport.Height)
		model.viewport.SetYOffset(min(max(0, startOfLatest), maxOffset))
	}
}

func (model Model) renderMessage(item message) string {
	contentWidth := max(20, model.viewport.Width-2)
	bodyWidth := max(16, contentWidth-4)
	if item.role == "user" {
		label := lipgloss.NewStyle().Foreground(accentSoft).Bold(true).Render("› You")
		body := lipgloss.NewStyle().Foreground(textColor).Width(bodyWidth).PaddingLeft(2).Render(terminalText(item.reply.Text))
		return label + "\n" + body
	}
	labelColor := accent
	label := "● fanqie"
	if item.role == "error" {
		labelColor = errorColor
		label = "● error"
	}
	header := lipgloss.NewStyle().Foreground(labelColor).Bold(true).Render(label)
	body := renderReplyWithStyle(item.reply, bodyWidth, model.readingStyle)
	return header + "\n" + lipgloss.NewStyle().PaddingLeft(2).Width(bodyWidth).Render(body)
}

func renderReply(reply agent.Reply, width int) string {
	return renderReplyWithStyle(reply, width, readingStyleStandard)
}

func renderReplyWithStyle(reply agent.Reply, width int, readingStyle readingStyle) string {
	reply = sanitizeReply(reply)
	title := ""
	if reply.Title != "" {
		title = lipgloss.NewStyle().Foreground(textColor).Bold(true).Render(reply.Title) + "\n"
	}
	muted := lipgloss.NewStyle().Foreground(mutedColor)
	regular := lipgloss.NewStyle().Foreground(textColor)

	switch reply.Kind {
	case agent.KindSearch:
		lines := []string{title}
		for index, book := range reply.Books {
			meta := strings.Join(nonEmpty(book.Author, book.Category, book.Status, score(book.Score), metric(book.ReadCount, "阅读")), " · ")
			selector := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(fmt.Sprintf("%02d", index+1))
			divider := muted.Render("│")
			lines = append(lines, fmt.Sprintf("%s %s %s", selector, divider, regular.Bold(true).Render(book.Title)))
			if meta != "" {
				lines = append(lines, "   "+divider+" "+muted.Render(meta))
			}
		}
		if reply.Text != "" {
			lines = append(lines, "", muted.Render(reply.Text))
		}
		return strings.Join(lines, "\n")
	case agent.KindBook:
		if reply.Book == nil {
			return title
		}
		book := reply.Book
		metadata := strings.Join(nonEmpty(
			book.Author,
			book.Category,
			book.Status,
			score(book.Score),
			metric(book.ReadCount, "阅读"),
			metric(book.BookshelfCount, "加书架"),
			compactNumber(book.WordCount)+"字",
			count(book.ChapterCount, "章"),
		), " · ")
		text := title + regular.Render(book.Title)
		if metadata != "" {
			text += "\n" + muted.Render(metadata)
		}
		if book.Abstract != "" {
			text += "\n\n" + regular.Render(typesetChineseProse(book.Abstract, width))
		}
		return text
	case agent.KindCatalog:
		lines := []string{title}
		for _, chapter := range reply.Chapters {
			lock := ""
			if chapter.Locked || chapter.NeedPay {
				lock = muted.Render("  [锁定]")
			}
			lines = append(lines, fmt.Sprintf("%4d  %s%s", chapter.Order, chapter.Title, lock))
		}
		if reply.Text != "" {
			lines = append(lines, "", muted.Render(reply.Text))
		}
		return strings.Join(lines, "\n")
	case agent.KindChapter:
		if reply.Chapter == nil {
			return title
		}
		chapter := reply.Chapter
		chapterTitle := chapter.Title
		if chapter.Order > 0 && !strings.HasPrefix(chapterTitle, "第") {
			chapterTitle = fmt.Sprintf("第%d章　%s", chapter.Order, chapterTitle)
		}
		content := typesetChineseProse(chapter.Content, width)
		chapterStyle := regular
		if readingStyle == readingStyleBold {
			chapterStyle = chapterStyle.Bold(true)
		}
		if readingStyle == readingStyleRelaxed {
			content = addReadingLineSpacing(content)
		}
		return lipgloss.NewStyle().Foreground(textColor).Bold(true).Render(chapterTitle) + "\n\n" + chapterStyle.Render(content) + "\n\n" + muted.Render("按 ←/→ 切换，或输入“下一章”（向前可输入“上一章”）")
	default:
		return title + regular.Width(width).Render(reply.Text)
	}
}

func addReadingLineSpacing(content string) string {
	lines := strings.Split(content, "\n")
	spaced := make([]string, 0, len(lines)*2)
	for index, line := range lines {
		spaced = append(spaced, line)
		if line != "" && index+1 < len(lines) && lines[index+1] != "" {
			spaced = append(spaced, "")
		}
	}
	return strings.Join(spaced, "\n")
}

// View lays out the message stream, composer and status line.
func (model Model) View() string {
	if !model.ready {
		return "正在启动番茄阅读助手…"
	}
	if model.width < 24 || model.height < 8 {
		width := max(1, model.width)
		message := "窗口太小，请扩大终端"
		if model.width >= 16 && model.height >= 4 {
			message = "窗口太小\n至少需要 24 列 × 8 行\nCtrl+C 退出"
		}
		return lipgloss.NewStyle().Foreground(mutedColor).Width(width).MaxWidth(width).Render(message)
	}
	contentWidth := max(20, model.width-4)
	header := model.headerView(contentWidth)

	composerBorder := accentSoft
	if model.busy {
		composerBorder = borderColor
	}
	composer := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(composerBorder).
		Padding(0, 1).
		Width(contentWidth - 2).
		Render(model.input.View())

	matches := model.slashCommandMatches()
	left := "● " + model.status
	if model.commandPanel {
		left = "● 快捷面板"
	}
	right := "F1 命令面板  ·  Enter 发送  ·  ←/→ 章节  ·  F2 正文样式  ·  PgUp/PgDn 滚动  ·  Ctrl+C 退出"
	if len(matches) > 0 {
		left = fmt.Sprintf("● %d 个匹配命令", len(matches))
		right = "↑/↓ 选择 · Tab/Enter 补全 · Esc 收起"
	}
	if lipgloss.Width(left)+1+lipgloss.Width(right) > contentWidth {
		if len(matches) > 0 {
			right = "↑↓ · Tab · Esc"
		} else {
			right = "F1 面板 · ←/→ 章节 · F2 样式 · Enter"
		}
	}
	footer := lipgloss.NewStyle().Foreground(mutedColor).Width(contentWidth).Render(fitStatusRow(left, right, contentWidth))

	body := model.viewport.View()
	if model.commandPanel {
		body = model.commandPanelView(contentWidth, model.viewport.Height)
	} else if len(matches) > 0 {
		menu := model.slashCommandMenuView(matches, contentWidth, model.viewport.Height)
		menuHeight := lipgloss.Height(menu)
		if remaining := model.viewport.Height - menuHeight; remaining >= 1 {
			conversation := model.viewport
			conversation.Height = remaining
			body = lipgloss.JoinVertical(lipgloss.Left, conversation.View(), menu)
		} else {
			body = menu
		}
	}

	return lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.NewStyle().MaxHeight(max(1, model.height-2)).Render(
			lipgloss.JoinVertical(lipgloss.Left, header, body, composer, footer),
		),
	)
}

func (model Model) headerView(width int) string {
	brand := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("番茄阅读")
	modeText := "○ 公开阅读"
	connected := false
	if model.agent != nil {
		if controller, ok := model.agent.Provider.(fanqie.SessionController); ok && controller.HasSession() {
			connected = true
			modeText = "● 官网同步"
		}
	}
	if width >= 48 {
		if connected {
			modeText += " · 书架与进度"
		} else {
			modeText += " · 可选账号"
		}
	}
	modeColor := mutedColor
	if connected {
		modeColor = accentSoft
	}
	mode := lipgloss.NewStyle().Foreground(modeColor).Render(modeText)
	top := fitStatusRow(brand, mode, width)

	context := lipgloss.NewStyle().Foreground(mutedColor).Render(model.contextLabel())
	progress := ""
	if model.chapterIndex > 0 && model.chapterTotal > 0 && width >= 48 {
		progress = lipgloss.NewStyle().Foreground(accentSoft).Render(readingProgress(model.chapterIndex, model.chapterTotal, 10))
	}
	return top + "\n" + fitStatusRow(context, progress, width)
}

func fitStatusRow(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	right = ansi.Truncate(right, max(0, width-1), "")
	maxLeftWidth := max(0, width-lipgloss.Width(right)-1)
	left = ansi.Truncate(left, maxLeftWidth, "…")
	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func readingProgress(position, total, size int) string {
	if position <= 0 || total <= 0 || size <= 0 {
		return ""
	}
	position = min(position, total)
	filled := max(1, position*size/total)
	filled = min(filled, size)
	return strings.Repeat("▰", filled) + strings.Repeat("▱", size-filled) + fmt.Sprintf(" %d/%d", position, total)
}

func (model Model) commandPanelView(width, height int) string {
	innerWidth := max(1, width-4)
	innerHeight := max(1, height-2)
	lines := []string{
		lipgloss.NewStyle().Foreground(accent).Bold(true).Render("命令面板"),
		"搜索与发现  /search 书名  /popular  /categories",
		"账号与书架  /sync  /bookshelf  /favorite  /unfavorite",
		"阅读与记录  /catalog  /history  /resume  /cloud-history",
		"书籍与评价  /reviews  /review-feed  /comment 链接",
		"",
		"←/→ 切章   F2 正文样式   PgUp/PgDn 滚动",
		"F1 / Ctrl+K 关闭面板   Esc 返回",
	}
	if innerHeight < 8 {
		lines = []string{
			lipgloss.NewStyle().Foreground(accent).Bold(true).Render("命令面板"),
			"/search  /popular  /sync  /bookshelf",
			"←/→ 切章 · F2 样式 · F1 关闭",
		}
	}
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	for index := range lines {
		lines[index] = ansi.Truncate(lines[index], innerWidth, "")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentSoft).
		Padding(0, 1).
		Width(max(1, width-2)).
		Height(innerHeight).
		Render(strings.Join(lines, "\n"))
}

func (model Model) slashCommandMenuView(matches []slashCommand, width, height int) string {
	if len(matches) == 0 || width <= 0 || height <= 0 {
		return ""
	}
	selected := model.normalizedSlashSelection(len(matches))
	if height <= 3 {
		command := matches[selected]
		label := command.name
		if command.arguments != "" {
			label += " " + command.arguments
		}
		lines := []string{
			lipgloss.NewStyle().Foreground(accent).Bold(true).Render("› " + ansi.Truncate(label, max(1, width-2), "…")),
		}
		if height >= 2 {
			lines = append(lines, lipgloss.NewStyle().Foreground(mutedColor).Render(ansi.Truncate(command.description, width, "…")))
		}
		if height >= 3 {
			lines = append(lines, lipgloss.NewStyle().Foreground(mutedColor).Render(ansi.Truncate("↑↓ 选择 · Tab 补全 · Esc 收起", width, "")))
		}
		return strings.Join(lines, "\n")
	}

	innerWidth := max(1, width-4)
	maxItems := max(1, min(6, height-4))
	start := max(0, selected-maxItems+1)
	end := min(len(matches), start+maxItems)
	if end-start < maxItems {
		start = max(0, end-maxItems)
	}
	lines := []string{lipgloss.NewStyle().Foreground(mutedColor).Render(fmt.Sprintf("/  命令补全 · %d 个匹配", len(matches)))}
	for index := start; index < end; index++ {
		command := matches[index]
		label := command.name
		if command.arguments != "" {
			label += " " + command.arguments
		}
		marker := "  "
		style := lipgloss.NewStyle().Foreground(textColor)
		if index == selected {
			marker = "› "
			style = style.Foreground(accent).Bold(true)
		}
		commandWidth := max(8, min(34, innerWidth-10))
		left := style.Render(marker + ansi.Truncate(label, commandWidth, "…"))
		right := ""
		if innerWidth >= 44 {
			right = lipgloss.NewStyle().Foreground(mutedColor).Render(command.description)
		}
		lines = append(lines, fitStatusRow(left, right, innerWidth))
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(mutedColor).Render("↑/↓ 选择  ·  Tab/Enter 补全  ·  Esc 收起"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentSoft).
		Padding(0, 1).
		Width(max(1, width-2)).
		Render(strings.Join(lines, "\n"))
}

func (model Model) contextLabel() string {
	if model.bookTitle == "" {
		return "未选择书籍 · 正文" + model.readingStyle.label()
	}
	label := terminalText(model.bookTitle)
	if model.chapterIndex > 0 && model.chapterTotal > 0 {
		label += fmt.Sprintf(" · %d/%d", model.chapterIndex, model.chapterTotal)
	}
	return label + " · 正文" + model.readingStyle.label()
}

func sanitizeReply(reply agent.Reply) agent.Reply {
	reply.Title = terminalText(reply.Title)
	reply.Text = terminalText(reply.Text)
	if reply.Book != nil {
		book := *reply.Book
		sanitizeBook(&book)
		reply.Book = &book
	}
	if len(reply.Books) > 0 {
		reply.Books = append([]fanqie.Book(nil), reply.Books...)
		for index := range reply.Books {
			sanitizeBook(&reply.Books[index])
		}
	}
	if len(reply.Chapters) > 0 {
		reply.Chapters = append([]fanqie.Chapter(nil), reply.Chapters...)
		for index := range reply.Chapters {
			reply.Chapters[index].Title = terminalText(reply.Chapters[index].Title)
			reply.Chapters[index].Volume = terminalText(reply.Chapters[index].Volume)
		}
	}
	if reply.Chapter != nil {
		chapter := *reply.Chapter
		chapter.Title = terminalText(chapter.Title)
		chapter.Content = terminalText(chapter.Content)
		chapter.BookName = terminalText(chapter.BookName)
		reply.Chapter = &chapter
	}
	return reply
}

func sanitizeBook(book *fanqie.Book) {
	book.Title = terminalText(book.Title)
	book.Author = terminalText(book.Author)
	book.Abstract = terminalText(book.Abstract)
	book.Category = terminalText(book.Category)
	book.Status = terminalText(book.Status)
}

func terminalText(value string) string {
	value = ansi.Strip(value)
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' || !unicode.IsControl(character) {
			return character
		}
		return -1
	}, value)
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && value != "0字" && value != "0章" {
			result = append(result, value)
		}
	}
	return result
}

func score(value float64) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%g 分", value)
}

func count(value int, suffix string) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d%s", value, suffix)
}

func metric(value int, label string) string {
	if value <= 0 {
		return ""
	}
	return compactNumber(value) + " " + label
}

func compactNumber(value int) string {
	switch {
	case value >= 100_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(value)/100_000_000), ".0") + "亿"
	case value >= 10_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(value)/10_000), ".0") + "万"
	default:
		return fmt.Sprintf("%d", value)
	}
}
