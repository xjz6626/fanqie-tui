package agent

import (
	"context"
	"testing"

	"github.com/xjz6626/fanqie-tui/internal/fanqie"
)

type fakeProvider struct{}

func (fakeProvider) Search(_ context.Context, query string, _ int) (fanqie.SearchPage, error) {
	return fanqie.SearchPage{Books: []fanqie.Book{{ID: "book-1", Title: query + "结果", Author: "作者"}}}, nil
}

func (fakeProvider) GetBook(_ context.Context, id string) (fanqie.Book, error) {
	return fanqie.Book{ID: id, Title: "示例书", Author: "作者", ChapterCount: 3}, nil
}

func (fakeProvider) GetDirectory(context.Context, string) ([]fanqie.Chapter, error) {
	return []fanqie.Chapter{
		{ID: "chapter-1", Title: "第1章 开篇", Order: 1},
		{ID: "chapter-2", Title: "第2章 后续", Order: 2},
		{ID: "chapter-3", Title: "第3章 锁定", Order: 3, Locked: true},
	}, nil
}

func (fakeProvider) GetChapter(_ context.Context, id string) (fanqie.ChapterContent, error) {
	order := 1
	if id == "chapter-2" {
		order = 2
	}
	return fanqie.ChapterContent{BookID: "book-1", ChapterID: id, Title: "正文标题", Order: order, Content: "章节正文"}, nil
}

func TestConversationalFlow(t *testing.T) {
	state := New(fakeProvider{})
	ctx := context.Background()

	replies, err := state.Handle(ctx, "搜索 示例")
	if err != nil || len(replies) != 1 || replies[0].Kind != KindSearch {
		t.Fatalf("search: replies=%+v err=%v", replies, err)
	}
	replies, err = state.Handle(ctx, "打开 1")
	if err != nil || state.Book == nil || len(state.Chapters) != 3 || replies[0].Kind != KindBook {
		t.Fatalf("open: replies=%+v err=%v", replies, err)
	}
	replies, err = state.Handle(ctx, "从第 1 章开始读")
	if err != nil || replies[0].Kind != KindChapter || state.Index != 0 {
		t.Fatalf("read: replies=%+v err=%v", replies, err)
	}
	replies, err = state.Handle(ctx, "下一章")
	if err != nil || replies[0].Chapter == nil || replies[0].Chapter.Order != 2 || state.Index != 1 {
		t.Fatalf("next: replies=%+v err=%v", replies, err)
	}
	replies, err = state.Handle(ctx, "上一章")
	if err != nil || state.Index != 0 {
		t.Fatalf("prev: replies=%+v err=%v", replies, err)
	}
}

func TestBarePromptSearchesBeforeBookIsOpen(t *testing.T) {
	state := New(fakeProvider{})
	replies, err := state.Handle(context.Background(), "三体")
	if err != nil || replies[0].Books[0].Title != "三体结果" {
		t.Fatalf("replies=%+v err=%v", replies, err)
	}
}

func TestLockedChapterDoesNotAdvanceProgress(t *testing.T) {
	state := New(fakeProvider{})
	ctx := context.Background()
	_, _ = state.Handle(ctx, "示例")
	_, _ = state.Handle(ctx, "打开 1")
	replies, err := state.Handle(ctx, "第 3 章")
	if err != nil || replies[0].Kind != KindText || state.Index != 2 {
		t.Fatalf("replies=%+v index=%d err=%v", replies, state.Index, err)
	}
}

func TestHelpStatusAndClear(t *testing.T) {
	state := New(fakeProvider{})
	ctx := context.Background()
	for _, input := range []string{"帮助", "/status", "/clear"} {
		replies, err := state.Handle(ctx, input)
		if err != nil || len(replies) != 1 {
			t.Fatalf("%s: replies=%+v err=%v", input, replies, err)
		}
	}
}
