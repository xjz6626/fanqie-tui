package fanqie

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPublicWebsiteIntegration(t *testing.T) {
	if os.Getenv("FANQIE_INTEGRATION") != "1" {
		t.Skip("set FANQIE_INTEGRATION=1 to test the live public website")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	provider := NewWebProvider(45 * time.Second)

	page, err := provider.Search(ctx, "斗罗大陆", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Books) == 0 || page.Books[0].ID == "" {
		t.Fatalf("search returned no usable books: %+v", page)
	}

	const bookID = "7553142351103806488"
	book, err := provider.GetBook(ctx, bookID)
	if err != nil {
		t.Fatal(err)
	}
	if book.Title == "" || book.ChapterCount == 0 {
		t.Fatalf("book metadata is incomplete: %+v", book)
	}
	chapters, err := provider.GetDirectory(ctx, bookID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) == 0 {
		t.Fatal("directory is empty")
	}
	chapter, err := provider.GetChapter(ctx, "7553142907742470680")
	if err != nil {
		t.Fatal(err)
	}
	if chapter.Content == "" || strings.ContainsAny(chapter.Content, "\ue3e8\ue3e9\ue3ea") {
		t.Fatal("chapter content was not decoded")
	}
}
