"""Provider contract used by the CLI and reader."""

from __future__ import annotations

from abc import ABC, abstractmethod

from .models import Book, Chapter, ChapterContent, SearchPage


class NovelProvider(ABC):
    """A replaceable upstream source."""

    @abstractmethod
    def search(self, query: str, offset: int = 0) -> SearchPage:
        raise NotImplementedError

    @abstractmethod
    def get_book(self, book_id: str) -> Book:
        raise NotImplementedError

    @abstractmethod
    def get_directory(self, book_id: str) -> list[Chapter]:
        raise NotImplementedError

    @abstractmethod
    def get_chapter(self, item_id: str) -> ChapterContent:
        raise NotImplementedError
