package fanqie

import "context"

// Provider is the data source contract used by the local reading agent.
type Provider interface {
	Search(context.Context, string, int) (SearchPage, error)
	GetBook(context.Context, string) (Book, error)
	GetDirectory(context.Context, string) ([]Chapter, error)
	GetChapter(context.Context, string) (ChapterContent, error)
}
