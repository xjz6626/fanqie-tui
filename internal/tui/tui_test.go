package tui

import (
	"fmt"
	"strings"
	"testing"

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

func TestNewModelFocusesComposer(t *testing.T) {
	model := New(agent.New(nil), 0, "")
	if !model.input.Focused() {
		t.Fatal("composer should be focused at startup")
	}
}

func TestRenderSearchReplyLimitsFirstScreen(t *testing.T) {
	books := make([]fanqie.Book, 8)
	for index := range books {
		books[index] = fanqie.Book{Title: fmt.Sprintf("书籍 %d", index+1)}
	}
	rendered := renderReply(agent.Reply{Kind: agent.KindSearch, Books: books}, 60)
	if strings.Contains(rendered, "书籍 6") || !strings.Contains(rendered, "另有 3 项结果") {
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

func TestCompactNumber(t *testing.T) {
	for value, expected := range map[int]string{0: "0", 12_345: "1.2万", 100_000: "10万", 123_000_000: "1.2亿"} {
		if got := compactNumber(value); got != expected {
			t.Fatalf("compactNumber(%d)=%q, want %q", value, got, expected)
		}
	}
}
