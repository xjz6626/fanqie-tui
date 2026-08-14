package fanqie

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xjz6626/fanqie-tui/internal/session"
)

func TestPublicWebsiteIntegration(t *testing.T) {
	if os.Getenv("FANQIE_INTEGRATION") != "1" {
		t.Skip("set FANQIE_INTEGRATION=1 to test the live public website")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	provider := NewWebProvider(45 * time.Second)
	for _, kind := range []DiscoverKind{DiscoverPopular, DiscoverRecommended, DiscoverMale, DiscoverFemale, DiscoverRecent, DiscoverPublished} {
		discovered, err := provider.Discover(ctx, kind, 0)
		if err != nil {
			t.Fatalf("discover %s: %v", kind, err)
		}
		if len(discovered.Books) == 0 || discovered.Books[0].ID == "" {
			t.Fatalf("discover %s returned no usable books: %+v", kind, discovered)
		}
	}
	categories, err := provider.GetCategories(ctx)
	if err != nil || len(categories) == 0 {
		t.Fatalf("categories=%d err=%v", len(categories), err)
	}
	categoryPage, err := provider.GetCategoryRank(ctx, "1139", "female", 0)
	if err != nil || len(categoryPage.Books) == 0 {
		t.Fatalf("female category rank=%+v err=%v", categoryPage, err)
	}
	authors, err := provider.GetTopAuthors(ctx)
	if err != nil || len(authors) == 0 {
		t.Fatalf("authors=%d err=%v", len(authors), err)
	}
	profile, works, err := provider.GetAuthor(ctx, authors[0].ID)
	if err != nil || profile.Name == "" || len(works) == 0 {
		t.Fatalf("author profile=%+v works=%d err=%v", profile, len(works), err)
	}
	if _, err := provider.GetAccount(ctx); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("anonymous account check error = %v, want ErrLoginRequired", err)
	}

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
	statsBook, err := provider.GetBook(ctx, "7380914885728668734")
	if err != nil || statsBook.Score <= 0 || statsBook.ReadCount <= 0 {
		t.Fatalf("book score/read count missing: book=%+v err=%v", statsBook, err)
	}
	feed, err := provider.GetReviewFeed(ctx)
	if err != nil || len(feed) == 0 || feed[0].BookID == "" || feed[0].CommentID == "" {
		t.Fatalf("review feed=%d first=%+v err=%v", len(feed), firstReview(feed), err)
	}
	comment, err := provider.GetComment(ctx, "7380914885728668734", "7437128617000010520")
	if err != nil || comment.Book.Score <= 0 || len(comment.Reviews) == 0 || comment.Reviews[0].Text == "" {
		t.Fatalf("comment detail=%+v err=%v", comment, err)
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

func firstReview(reviews []ReviewSummary) ReviewSummary {
	if len(reviews) == 0 {
		return ReviewSummary{}
	}
	return reviews[0]
}

// TestAuthenticatedWebsiteIntegration only performs read operations. The
// bookshelf metadata endpoint is a POST but does not modify account state. The
// test is isolated because it needs a real browser session and depends on
// private website response shapes.
func TestAuthenticatedWebsiteIntegration(t *testing.T) {
	if os.Getenv("FANQIE_AUTH_INTEGRATION") != "1" {
		t.Skip("set FANQIE_AUTH_INTEGRATION=1 and FANQIE_COOKIE_FILE to test authenticated read-only APIs")
	}
	cookieFile := strings.TrimSpace(os.Getenv("FANQIE_COOKIE_FILE"))
	if cookieFile == "" {
		t.Fatal("FANQIE_COOKIE_FILE is required")
	}
	cookie, err := session.LoadCookieFile(cookieFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	provider := NewWebProviderWithSession(30*time.Second, cookie)
	account, err := provider.GetAccount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if account.ID == "" {
		t.Fatal("authenticated account is empty")
	}
	progress, err := provider.GetCloudProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range progress {
		if item.BookID == "" || (item.ProgressKnown && (item.ReadProgress < 0 || item.ReadProgress > 100)) {
			t.Fatalf("invalid cloud progress row: %+v", item)
		}
	}
	bookID := strings.TrimSpace(os.Getenv("FANQIE_AUTH_BOOK_ID"))
	if bookID == "" {
		bookID = "7553142351103806488"
	}
	items, err := provider.GetReadItems(ctx, bookID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ChapterID == "" || (item.ProgressKnown && (item.ReadProgress < 0 || item.ReadProgress > 100)) {
			t.Fatalf("invalid read item row: %+v", item)
		}
	}
	books, err := provider.GetBookshelf(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, book := range books {
		if book.ID == "" || book.Title == "" {
			t.Fatalf("invalid bookshelf row: %+v", book)
		}
	}
}

// TestAuthenticatedWebsiteWriteIntegration intentionally changes the real
// account: it exercises add, remove and progress writes against a stable
// public book. The original bookshelf state is restored before the test exits.
func TestAuthenticatedWebsiteWriteIntegration(t *testing.T) {
	if os.Getenv("FANQIE_AUTH_WRITE_INTEGRATION") != "1" {
		t.Skip("set FANQIE_AUTH_WRITE_INTEGRATION=1 and FANQIE_COOKIE_FILE to test authenticated writes")
	}
	cookieFile := strings.TrimSpace(os.Getenv("FANQIE_COOKIE_FILE"))
	if cookieFile == "" {
		t.Fatal("FANQIE_COOKIE_FILE is required")
	}
	cookie, err := session.LoadCookieFile(cookieFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	provider := NewWebProviderWithSession(30*time.Second, cookie)
	const bookID = "7380914885728668734"
	book, err := provider.GetBook(ctx, bookID)
	if err != nil {
		t.Fatal(err)
	}
	originallyInShelf, err := provider.InBookshelf(ctx, bookID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer restoreCancel()
		if originallyInShelf {
			if restoreErr := provider.AddToBookshelf(restoreCtx, book); restoreErr != nil {
				t.Errorf("restore original bookshelf state: %v", restoreErr)
			}
		} else if restoreErr := provider.RemoveFromBookshelf(restoreCtx, book); restoreErr != nil {
			t.Errorf("restore original bookshelf state: %v", restoreErr)
		}
	}()
	if err := provider.AddToBookshelf(ctx, book); err != nil {
		t.Fatal(err)
	}
	inShelf, err := provider.InBookshelf(ctx, bookID)
	if err != nil || !inShelf {
		t.Fatalf("official bookshelf did not reflect add: in=%t err=%v", inShelf, err)
	}
	if err := provider.RemoveFromBookshelf(ctx, book); err != nil {
		t.Fatal(err)
	}
	inShelf, err = provider.InBookshelf(ctx, bookID)
	if err != nil || inShelf {
		t.Fatalf("official bookshelf did not reflect delete: in=%t err=%v", inShelf, err)
	}
	if err := provider.AddToBookshelf(ctx, book); err != nil {
		t.Fatal(err)
	}
	chapters, err := provider.GetDirectory(ctx, bookID)
	if err != nil || len(chapters) == 0 {
		t.Fatalf("directory=%d err=%v", len(chapters), err)
	}
	chapter := chapters[0]
	if err := provider.UpdateProgress(ctx, bookID, chapter.ID, chapter.Order); err != nil {
		t.Fatal(err)
	}
	progress, err := provider.GetCloudProgress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundProgress := false
	for _, item := range progress {
		if item.BookID == bookID && item.ChapterID == chapter.ID {
			foundProgress = true
			break
		}
	}
	if !foundProgress {
		t.Fatal("official cloud progress did not reflect the chapter write")
	}
}
