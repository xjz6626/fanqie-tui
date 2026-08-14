package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/xjz6626/fanqie-tui/internal/fanqie"
)

func TestDefaultPathUsesXDGStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(stateHome, "fanqie-tui", "library.json")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestDefaultPathIgnoresRelativeXDGStateHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "relative-state")

	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "state", "fanqie-tui", "library.json")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestOpenMissingUsesDefaultsWithoutCreatingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "library.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot(); got.Version != CurrentVersion || got.Settings != DefaultReadingSettings() {
		t.Fatalf("unexpected defaults: %#v", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open created the file or returned wrong error: %v", err)
	}
}

func TestStorePersistsHistoryFavoritesAndSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "library.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	firstTime := time.Date(2026, 8, 12, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	store.now = func() time.Time { return firstTime }
	book := testBook("book-1")
	chapter := testChapter("chapter-3", 3)
	if err := store.RecordHistory(book, chapter, 2); err != nil {
		t.Fatal(err)
	}
	added, err := store.AddFavorite(book)
	if err != nil || !added {
		t.Fatalf("AddFavorite() = %v, %v", added, err)
	}
	settings := ReadingSettings{FontStyle: FontStyleBold, LineSpacing: 1, ParagraphIndent: 4}
	if err := store.SetSettings(settings); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file permissions = %o, want 600", got)
	}
	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := parentInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", got)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Snapshot()
	if got.Version != CurrentVersion || got.Settings != settings {
		t.Fatalf("metadata mismatch after reopen: %#v", got)
	}
	if len(got.History) != 1 || got.History[0].Book.ID != book.ID || got.History[0].ChapterIndex != 2 {
		t.Fatalf("history mismatch: %#v", got.History)
	}
	if !got.History[0].ReadAt.Equal(firstTime.UTC()) {
		t.Fatalf("ReadAt = %v, want %v", got.History[0].ReadAt, firstTime.UTC())
	}
	if len(got.Favorites) != 1 || got.Favorites[0].Book.ID != book.ID {
		t.Fatalf("favorites mismatch: %#v", got.Favorites)
	}
}

func TestRecordHistoryDeduplicatesSortsAndLimits(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.json"), WithHistoryLimit(2))
	if err != nil {
		t.Fatal(err)
	}

	clock := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time {
		clock = clock.Add(time.Minute)
		return clock
	}
	for _, id := range []string{"one", "two", "one", "three"} {
		if err := store.RecordHistory(testBook(id), testChapter("chapter-"+id, 1), 0); err != nil {
			t.Fatal(err)
		}
	}

	history := store.History()
	if len(history) != 2 || history[0].Book.ID != "three" || history[1].Book.ID != "one" {
		t.Fatalf("unexpected history: %#v", history)
	}
	latest, ok := store.LatestHistory()
	if !ok || latest.Book.ID != "three" {
		t.Fatalf("LatestHistory() = %#v, %v", latest, ok)
	}
	one, ok := store.HistoryFor(" one ")
	if !ok || one.Book.ID != "one" {
		t.Fatalf("HistoryFor(one) = %#v, %v", one, ok)
	}
	if _, ok := store.HistoryFor("two"); ok {
		t.Fatal("limited history unexpectedly contains book two")
	}
}

func TestFavoritesDeduplicateRefreshRemoveAndLimit(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.json"), WithFavoriteLimit(2))
	if err != nil {
		t.Fatal(err)
	}

	clock := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time {
		clock = clock.Add(time.Minute)
		return clock
	}
	for _, id := range []string{"one", "two", "three"} {
		added, err := store.AddFavorite(testBook(id))
		if err != nil || !added {
			t.Fatalf("AddFavorite(%q) = %v, %v", id, added, err)
		}
	}
	if got := store.Favorites(); len(got) != 2 || got[0].Book.ID != "three" || got[1].Book.ID != "two" {
		t.Fatalf("unexpected limited favorites: %#v", got)
	}

	updated := testBook("two")
	updated.Title = "updated title"
	added, err := store.AddFavorite(updated)
	if err != nil || added {
		t.Fatalf("duplicate AddFavorite() = %v, %v", added, err)
	}
	if !store.IsFavorite(" two ") {
		t.Fatal("IsFavorite(two) = false")
	}
	got := store.Favorites()
	if got[1].Book.Title != updated.Title {
		t.Fatalf("favorite metadata was not refreshed: %#v", got[1])
	}

	removed, err := store.RemoveFavorite(" three ")
	if err != nil || !removed {
		t.Fatalf("RemoveFavorite(three) = %v, %v", removed, err)
	}
	removed, err = store.RemoveFavorite("missing")
	if err != nil || removed {
		t.Fatalf("RemoveFavorite(missing) = %v, %v", removed, err)
	}
}

func TestLoadNormalizesUntrustedCollections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	old := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	newer := old.Add(time.Hour)
	data := State{
		Version:  CurrentVersion,
		Settings: DefaultReadingSettings(),
		History: []HistoryEntry{
			{Book: testBook("duplicate"), Chapter: testChapter("old", 1), ReadAt: old},
			{Book: testBook("duplicate"), Chapter: testChapter("new", 2), ChapterIndex: 1, ReadAt: newer},
			{Book: testBook(""), Chapter: testChapter("invalid", 1), ReadAt: newer},
		},
		Favorites: []Favorite{
			{Book: testBook("duplicate"), AddedAt: old},
			{Book: testBook("duplicate"), AddedAt: newer},
			{Book: testBook(""), AddedAt: newer},
		},
	}
	contents, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot()
	if len(got.History) != 1 || got.History[0].Chapter.ID != "new" {
		t.Fatalf("history was not normalized: %#v", got.History)
	}
	if len(got.Favorites) != 1 || !got.Favorites[0].AddedAt.Equal(newer) {
		t.Fatalf("favorites were not normalized: %#v", got.Favorites)
	}
}

func TestInvalidInputsDoNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	checks := []error{
		store.RecordHistory(fanqie.Book{}, testChapter("chapter", 1), 0),
		store.RecordHistory(testBook("book"), fanqie.Chapter{}, 0),
		store.RecordHistory(testBook("book"), testChapter("chapter", 1), -1),
		store.SetSettings(ReadingSettings{FontStyle: "serif"}),
		store.SetSettings(ReadingSettings{FontStyle: FontStyleRegular, LineSpacing: 4}),
		store.SetSettings(ReadingSettings{FontStyle: FontStyleRegular, ParagraphIndent: 9}),
	}
	if _, err := store.AddFavorite(fanqie.Book{}); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("empty favorite error = %v", err)
	}
	if _, err := store.RemoveFavorite(" "); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("empty removal error = %v", err)
	}
	for _, err := range checks {
		if !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("validation error = %v, want ErrInvalidRecord", err)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid mutations created the file: %v", err)
	}
}

func TestOpenRejectsCorruptAndUnsupportedData(t *testing.T) {
	t.Run("corrupt", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "library.json")
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil {
			t.Fatal("Open(corrupt) unexpectedly succeeded")
		}
	})

	t.Run("future version", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "library.json")
		contents := fmt.Appendf(nil, "{\"version\":%d}", CurrentVersion+1)
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatalf("Open(future version) error = %v", err)
		}
	})
}

func TestFailedSaveDoesNotChangeMemory(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	store, err := Open(filepath.Join(blocked, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AddFavorite(testBook("book")); err == nil {
		t.Fatal("AddFavorite unexpectedly succeeded")
	}
	if len(store.Favorites()) != 0 {
		t.Fatal("failed mutation changed in-memory favorites")
	}
}

func TestSnapshotIsDetached(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordHistory(testBook("book"), testChapter("chapter", 1), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddFavorite(testBook("book")); err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot()
	snapshot.History[0].Book.Title = "mutated"
	snapshot.Favorites[0].Book.Title = "mutated"
	if got := store.Snapshot(); got.History[0].Book.Title == "mutated" || got.Favorites[0].Book.Title == "mutated" {
		t.Fatal("Snapshot returned shared slice storage")
	}
}

func TestConcurrentMutations(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.json"), WithHistoryLimit(50), WithFavoriteLimit(50))
	if err != nil {
		t.Fatal(err)
	}

	const count = 20
	var wait sync.WaitGroup
	wait.Add(count)
	for index := range count {
		go func() {
			defer wait.Done()
			id := fmt.Sprintf("book-%02d", index)
			book := testBook(id)
			if err := store.RecordHistory(book, testChapter("chapter-"+id, 1), 0); err != nil {
				t.Errorf("RecordHistory(%q): %v", id, err)
			}
			if _, err := store.AddFavorite(book); err != nil {
				t.Errorf("AddFavorite(%q): %v", id, err)
			}
		}()
	}
	wait.Wait()
	if got := store.Snapshot(); len(got.History) != count || len(got.Favorites) != count {
		t.Fatalf("lost concurrent mutations: history=%d favorites=%d", len(got.History), len(got.Favorites))
	}
	if _, err := Open(store.Path()); err != nil {
		t.Fatalf("concurrent writes left invalid JSON: %v", err)
	}
}

func TestOptionsRejectNonPositiveLimits(t *testing.T) {
	for _, option := range []Option{WithHistoryLimit(0), WithFavoriteLimit(-1)} {
		if _, err := Open(filepath.Join(t.TempDir(), "library.json"), option); !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("Open() error = %v, want ErrInvalidRecord", err)
		}
	}
}

func testBook(id string) fanqie.Book {
	return fanqie.Book{ID: id, Title: "书 " + id, Author: "作者"}
}

func testChapter(id string, order int) fanqie.Chapter {
	return fanqie.Chapter{ID: id, Title: "章节 " + id, Order: order}
}
