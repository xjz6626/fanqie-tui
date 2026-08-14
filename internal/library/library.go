// Package library persists the reader's local history, favorites, and display
// preferences. It deliberately contains no account credentials or cookies.
package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xjz6626/fanqie-tui/internal/fanqie"
)

const (
	// CurrentVersion is the JSON schema version written by this package.
	CurrentVersion = 1

	// DefaultHistoryLimit keeps the state file useful without allowing it to
	// grow forever.
	DefaultHistoryLimit  = 200
	DefaultFavoriteLimit = 1000

	FontStyleRegular = "regular"
	FontStyleBold    = "bold"
)

var (
	ErrInvalidRecord      = errors.New("invalid library record")
	ErrUnsupportedVersion = errors.New("unsupported library data version")
)

// ReadingSettings contains presentation preferences that are meaningful
// inside a terminal. The terminal emulator remains responsible for selecting
// the actual font family and point size.
type ReadingSettings struct {
	FontStyle       string `json:"font_style"`
	LineSpacing     int    `json:"line_spacing"`
	ParagraphIndent int    `json:"paragraph_indent"`
}

// DefaultReadingSettings returns settings suitable for Chinese prose.
func DefaultReadingSettings() ReadingSettings {
	return ReadingSettings{
		FontStyle:       FontStyleRegular,
		LineSpacing:     0,
		ParagraphIndent: 2,
	}
}

// HistoryEntry records the latest reading position for one book.
type HistoryEntry struct {
	Book         fanqie.Book    `json:"book"`
	Chapter      fanqie.Chapter `json:"chapter"`
	ChapterIndex int            `json:"chapter_index"`
	ReadAt       time.Time      `json:"read_at"`
}

// Favorite contains a locally saved book and the time it was added.
type Favorite struct {
	Book    fanqie.Book `json:"book"`
	AddedAt time.Time   `json:"added_at"`
}

// State is the versioned representation stored on disk.
type State struct {
	Version   int             `json:"version"`
	History   []HistoryEntry  `json:"history"`
	Favorites []Favorite      `json:"favorites"`
	Settings  ReadingSettings `json:"settings"`
}

// Option customizes a Store.
type Option func(*Store) error

// WithHistoryLimit changes the maximum number of books kept in history.
func WithHistoryLimit(limit int) Option {
	return func(store *Store) error {
		if limit < 1 {
			return fmt.Errorf("history limit must be positive: %w", ErrInvalidRecord)
		}
		store.historyLimit = limit
		return nil
	}
}

// WithFavoriteLimit changes the maximum number of locally saved books.
func WithFavoriteLimit(limit int) Option {
	return func(store *Store) error {
		if limit < 1 {
			return fmt.Errorf("favorite limit must be positive: %w", ErrInvalidRecord)
		}
		store.favoriteLimit = limit
		return nil
	}
}

// Store is a goroutine-safe local reading library. A Store serializes its own
// writes; callers should share one Store instance within a process.
type Store struct {
	mu            sync.RWMutex
	path          string
	data          State
	historyLimit  int
	favoriteLimit int
	now           func() time.Time
}

// DefaultPath follows XDG_STATE_HOME when set to an absolute path and falls
// back to ~/.local/state/fanqie-tui/library.json.
func DefaultPath() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(stateHome) {
		return filepath.Join(stateHome, "fanqie-tui", "library.json"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home: %w", err)
	}
	if home == "" {
		return "", errors.New("locate user home: empty path")
	}
	return filepath.Join(home, ".local", "state", "fanqie-tui", "library.json"), nil
}

// OpenDefault opens the library at DefaultPath.
func OpenDefault(options ...Option) (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path, options...)
}

// Open loads a library file. A missing file produces an empty in-memory
// library and is created on the first mutation.
func Open(path string, options ...Option) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("library path is empty")
	}

	store := &Store{
		path:          path,
		historyLimit:  DefaultHistoryLimit,
		favoriteLimit: DefaultFavoriteLimit,
		now:           time.Now,
		data: State{
			Version:  CurrentVersion,
			Settings: DefaultReadingSettings(),
		},
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(store); err != nil {
			return nil, err
		}
	}

	data, err := load(path, store.historyLimit, store.favoriteLimit)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}
	store.data = data
	return store, nil
}

// Path returns the file backing this Store.
func (s *Store) Path() string {
	return s.path
}

// Snapshot returns a detached copy of all local library data.
func (s *Store) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.data)
}

// History returns newest reading positions first.
func (s *Store) History() []HistoryEntry {
	return s.Snapshot().History
}

// Favorites returns most recently added books first.
func (s *Store) Favorites() []Favorite {
	return s.Snapshot().Favorites
}

// LatestHistory returns the most recently read book, when one exists.
func (s *Store) LatestHistory() (HistoryEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.data.History) == 0 {
		return HistoryEntry{}, false
	}
	return s.data.History[0], true
}

// HistoryFor returns the saved reading position for a book.
func (s *Store) HistoryFor(bookID string) (HistoryEntry, bool) {
	bookID = strings.TrimSpace(bookID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entry := range s.data.History {
		if entry.Book.ID == bookID {
			return entry, true
		}
	}
	return HistoryEntry{}, false
}

// Settings returns the current reading preferences.
func (s *Store) Settings() ReadingSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Settings
}

// RecordHistory stores one latest reading position per book.
func (s *Store) RecordHistory(book fanqie.Book, chapter fanqie.Chapter, chapterIndex int) error {
	book.ID = strings.TrimSpace(book.ID)
	chapter.ID = strings.TrimSpace(chapter.ID)
	if book.ID == "" || chapter.ID == "" || chapterIndex < 0 {
		return fmt.Errorf("book id, chapter id, and non-negative chapter index are required: %w", ErrInvalidRecord)
	}

	return s.mutate(func(next *State) error {
		entry := HistoryEntry{
			Book:         book,
			Chapter:      chapter,
			ChapterIndex: chapterIndex,
			ReadAt:       s.now().UTC(),
		}
		history := make([]HistoryEntry, 0, len(next.History)+1)
		history = append(history, entry)
		for _, existing := range next.History {
			if existing.Book.ID != book.ID {
				history = append(history, existing)
			}
		}
		if len(history) > s.historyLimit {
			history = history[:s.historyLimit]
		}
		next.History = history
		return nil
	})
}

// AddFavorite adds or refreshes a favorite. It returns true only when the book
// was not already present; refreshing metadata preserves the original time.
func (s *Store) AddFavorite(book fanqie.Book) (bool, error) {
	book.ID = strings.TrimSpace(book.ID)
	if book.ID == "" {
		return false, fmt.Errorf("book id is required: %w", ErrInvalidRecord)
	}

	added := false
	err := s.mutate(func(next *State) error {
		for index := range next.Favorites {
			if next.Favorites[index].Book.ID == book.ID {
				next.Favorites[index].Book = book
				return nil
			}
		}
		next.Favorites = append([]Favorite{{Book: book, AddedAt: s.now().UTC()}}, next.Favorites...)
		if len(next.Favorites) > s.favoriteLimit {
			next.Favorites = next.Favorites[:s.favoriteLimit]
		}
		added = true
		return nil
	})
	return added, err
}

// RemoveFavorite removes a book by id and reports whether it existed.
func (s *Store) RemoveFavorite(bookID string) (bool, error) {
	bookID = strings.TrimSpace(bookID)
	if bookID == "" {
		return false, fmt.Errorf("book id is required: %w", ErrInvalidRecord)
	}

	removed := false
	err := s.mutate(func(next *State) error {
		favorites := next.Favorites[:0]
		for _, favorite := range next.Favorites {
			if favorite.Book.ID == bookID {
				removed = true
				continue
			}
			favorites = append(favorites, favorite)
		}
		next.Favorites = favorites
		return nil
	})
	return removed, err
}

// IsFavorite reports whether a book is saved locally.
func (s *Store) IsFavorite(bookID string) bool {
	bookID = strings.TrimSpace(bookID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, favorite := range s.data.Favorites {
		if favorite.Book.ID == bookID {
			return true
		}
	}
	return false
}

// SetSettings validates and persists reading display preferences.
func (s *Store) SetSettings(settings ReadingSettings) error {
	if err := validateSettings(settings); err != nil {
		return err
	}
	return s.mutate(func(next *State) error {
		next.Settings = settings
		return nil
	})
}

func (s *Store) mutate(change func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := cloneState(s.data)
	if err := change(&next); err != nil {
		return err
	}
	if err := save(s.path, next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func load(path string, historyLimit, favoriteLimit int) (State, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var data State
	if err := json.Unmarshal(contents, &data); err != nil {
		return State{}, fmt.Errorf("decode library %q: %w", path, err)
	}
	if data.Version != CurrentVersion {
		return State{}, fmt.Errorf("library %q has version %d, want %d: %w", path, data.Version, CurrentVersion, ErrUnsupportedVersion)
	}
	if err := validateSettings(data.Settings); err != nil {
		return State{}, fmt.Errorf("library %q: %w", path, err)
	}
	data.History = normalizeHistory(data.History, historyLimit)
	data.Favorites = normalizeFavorites(data.Favorites, favoriteLimit)
	return data, nil
}

func normalizeHistory(entries []HistoryEntry, limit int) []HistoryEntry {
	byBook := make(map[string]HistoryEntry, len(entries))
	for _, entry := range entries {
		entry.Book.ID = strings.TrimSpace(entry.Book.ID)
		entry.Chapter.ID = strings.TrimSpace(entry.Chapter.ID)
		if entry.Book.ID == "" || entry.Chapter.ID == "" || entry.ChapterIndex < 0 {
			continue
		}
		current, found := byBook[entry.Book.ID]
		if !found || entry.ReadAt.After(current.ReadAt) {
			byBook[entry.Book.ID] = entry
		}
	}

	result := make([]HistoryEntry, 0, len(byBook))
	for _, entry := range byBook {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ReadAt.Equal(result[j].ReadAt) {
			return result[i].Book.ID < result[j].Book.ID
		}
		return result[i].ReadAt.After(result[j].ReadAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func normalizeFavorites(entries []Favorite, limit int) []Favorite {
	byBook := make(map[string]Favorite, len(entries))
	for _, entry := range entries {
		entry.Book.ID = strings.TrimSpace(entry.Book.ID)
		if entry.Book.ID == "" {
			continue
		}
		current, found := byBook[entry.Book.ID]
		if !found || entry.AddedAt.After(current.AddedAt) {
			byBook[entry.Book.ID] = entry
		}
	}

	result := make([]Favorite, 0, len(byBook))
	for _, entry := range byBook {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AddedAt.Equal(result[j].AddedAt) {
			return result[i].Book.ID < result[j].Book.ID
		}
		return result[i].AddedAt.After(result[j].AddedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func validateSettings(settings ReadingSettings) error {
	if settings.FontStyle != FontStyleRegular && settings.FontStyle != FontStyleBold {
		return fmt.Errorf("unknown font style %q: %w", settings.FontStyle, ErrInvalidRecord)
	}
	if settings.LineSpacing < 0 || settings.LineSpacing > 3 {
		return fmt.Errorf("line spacing must be between 0 and 3: %w", ErrInvalidRecord)
	}
	if settings.ParagraphIndent < 0 || settings.ParagraphIndent > 8 {
		return fmt.Errorf("paragraph indent must be between 0 and 8: %w", ErrInvalidRecord)
	}
	return nil
}

func save(path string, data State) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create library directory %q: %w", directory, err)
	}

	contents, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode library: %w", err)
	}
	contents = append(contents, '\n')

	temporary, err := os.CreateTemp(directory, ".library-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary library file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set temporary library permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary library file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary library file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary library file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace library %q: %w", path, err)
	}

	// Best effort: the rename is atomic without this, while syncing the
	// directory improves durability after a sudden power loss on Unix.
	if directoryFile, err := os.Open(directory); err == nil {
		_ = directoryFile.Sync()
		_ = directoryFile.Close()
	}
	return nil
}

func cloneState(data State) State {
	clone := data
	clone.History = append([]HistoryEntry(nil), data.History...)
	clone.Favorites = append([]Favorite(nil), data.Favorites...)
	return clone
}
