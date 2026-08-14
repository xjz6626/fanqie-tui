package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTypesetChineseProseIndentsAndNormalizesParagraphs(t *testing.T) {
	input := "  第一段\u00a0内容  \n\n\n第二段\r\n"
	want := "　　第一段 内容\n\n　　第二段"
	if got := typesetChineseProse(input, 40); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapChineseLineObeysKinsoku(t *testing.T) {
	input := "天地玄黄，宇宙洪荒（测试）结束。"
	wrapped := wrapChineseLine(input, 10)
	for _, line := range strings.Split(wrapped, "\n") {
		if ansi.StringWidth(line) > 10 {
			t.Fatalf("line exceeds width: %q (%d)", line, ansi.StringWidth(line))
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if startsWithForbidden(trimmed, forbiddenLineStart) {
			t.Fatalf("forbidden punctuation at line start: %q", line)
		}
		last := []rune(trimmed)[len([]rune(trimmed))-1]
		if strings.ContainsRune(forbiddenLineEnd, last) {
			t.Fatalf("forbidden punctuation at line end: %q", line)
		}
	}
}

func TestWrapChineseLineKeepsLatinWordsWhenPossible(t *testing.T) {
	wrapped := wrapChineseLine("中文 BubbleTea 项目", 16)
	if strings.Contains(wrapped, "Bubble\nTea") {
		t.Fatalf("latin word was split unnecessarily: %q", wrapped)
	}

	long := wrapChineseLine("Supercalifragilistic", 8)
	for _, line := range strings.Split(long, "\n") {
		if ansi.StringWidth(line) > 8 {
			t.Fatalf("long word line exceeds width: %q", line)
		}
	}
}

func TestTypesetChineseProseKeepsEmojiClusters(t *testing.T) {
	wrapped := typesetChineseProse("你好👨‍👩‍👦，欢迎阅读。", 10)
	if !strings.Contains(wrapped, "👨‍👩‍👦") {
		t.Fatalf("emoji cluster was lost: %q", wrapped)
	}
	for _, line := range strings.Split(wrapped, "\n") {
		if ansi.StringWidth(line) > 10 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestWrapChineseLineHandlesVeryNarrowWidths(t *testing.T) {
	for width := 1; width <= 4; width++ {
		wrapped := wrapChineseLine("甲（乙），丙", width)
		if strings.HasPrefix(wrapped, "\n") || strings.Contains(wrapped, "\n\n") {
			t.Fatalf("width %d produced empty line: %q", width, wrapped)
		}
		for _, line := range strings.Split(wrapped, "\n") {
			if ansi.StringWidth(line) > max(width, 2) {
				t.Fatalf("width %d line is too wide: %q (%d)", width, line, ansi.StringWidth(line))
			}
		}
	}
}
