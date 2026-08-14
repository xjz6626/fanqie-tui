package fanqie

import "context"

// Provider is the data source contract used by the local reading agent.
type Provider interface {
	Search(context.Context, string, int) (SearchPage, error)
	Discover(context.Context, DiscoverKind, int) (SearchPage, error)
	GetBook(context.Context, string) (Book, error)
	GetDirectory(context.Context, string) ([]Chapter, error)
	GetChapter(context.Context, string) (ChapterContent, error)
}

// SessionProvider exposes optional official website features that require a
// user-supplied browser session. Provider remains usable without it.
type SessionProvider interface {
	GetAccount(context.Context) (Account, error)
}

// SessionController manages the in-memory browser session used by optional
// authenticated capabilities. Persistence belongs to the caller.
type SessionController interface {
	HasSession() bool
	SetSession(string)
	ClearSession()
}

// CloudProgressProvider exposes the account's read-only cloud reading
// positions. It is kept separate from SessionProvider so consumers can test
// and adopt each optional website capability independently.
type CloudProgressProvider interface {
	GetCloudProgress(context.Context) ([]CloudProgress, error)
}

// ReadItemsProvider exposes the read-only per-chapter positions for one book.
type ReadItemsProvider interface {
	GetReadItems(context.Context, string) ([]ReadItem, error)
}

// BookshelfProvider exposes the account's read-only official bookshelf.
type BookshelfProvider interface {
	GetBookshelf(context.Context) ([]Book, error)
}

// BookshelfController writes to the account's official bookshelf. Website
// support is intentionally split from the read-only provider capability.
type BookshelfController interface {
	InBookshelf(context.Context, string) (bool, error)
	AddToBookshelf(context.Context, Book) error
	RemoveFromBookshelf(context.Context, Book) error
}

// ProgressController synchronizes successful chapter reads to the official
// account. Implementations should be safe to call after the local read has
// already succeeded.
type ProgressController interface {
	UpdateProgress(context.Context, string, string, int) error
}

// CommentProvider exposes the official public review index and review pages.
type CommentProvider interface {
	GetReviewFeed(context.Context) ([]ReviewSummary, error)
	GetComment(context.Context, string, string) (CommentDetail, error)
}

// CategoryProvider exposes the public category catalog and category rankings.
type CategoryProvider interface {
	GetCategories(context.Context) ([]Category, error)
	GetCategoryRank(context.Context, string, string, int) (SearchPage, error)
}

// AuthorProvider exposes public highlighted writers and their works.
type AuthorProvider interface {
	GetTopAuthors(context.Context) ([]Author, error)
	GetAuthor(context.Context, string) (AuthorProfile, []Book, error)
}
