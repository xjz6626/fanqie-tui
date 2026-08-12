from __future__ import annotations

from click.testing import CliRunner

from fanqie_tui import cli
from fanqie_tui.errors import NetworkError
from fanqie_tui.models import Book, Chapter, ChapterContent, SearchPage


class FakeProvider:
    def __init__(self, timeout: float):
        self.timeout = timeout

    def __enter__(self) -> "FakeProvider":
        return self

    def __exit__(self, *_: object) -> None:
        pass

    def search(self, query: str, offset: int = 0) -> SearchPage:
        return SearchPage([Book("book-1", f"{query}结果", author="作者", status="连载")])

    def get_book(self, book_id: str) -> Book:
        return Book(book_id, "示例书", author="作者", chapter_count=2)

    def get_directory(self, book_id: str) -> list[Chapter]:
        return [Chapter("chapter-1", "开篇", 1), Chapter("chapter-2", "后续", 2)]

    def get_chapter(self, item_id: str) -> ChapterContent:
        order = 1 if item_id == "chapter-1" else 2
        return ChapterContent("book-1", item_id, "开篇" if order == 1 else "后续", "章节正文", order)


def test_help_lists_reader_commands() -> None:
    result = CliRunner().invoke(cli.main, ["--help"])

    assert result.exit_code == 0
    assert all(command in result.output for command in ("search", "catalog", "read", "browse"))


def test_search_output(monkeypatch) -> None:
    monkeypatch.setattr(cli, "WebProvider", FakeProvider)

    result = CliRunner().invoke(cli.main, ["search", "关键字"])

    assert result.exit_code == 0
    assert "关键字结果" in result.output
    assert "book-1" in result.output


def test_read_without_pager(monkeypatch) -> None:
    monkeypatch.setattr(cli, "WebProvider", FakeProvider)

    result = CliRunner().invoke(cli.main, ["read", "chapter-1", "--no-pager"])

    assert result.exit_code == 0
    assert "第 1 章" in result.output
    assert "章节正文" in result.output


def test_read_does_not_repeat_number_already_in_title(monkeypatch) -> None:
    class NumberedProvider(FakeProvider):
        def get_chapter(self, item_id: str) -> ChapterContent:
            return ChapterContent("book-1", item_id, "第1章 开篇", "章节正文", 1)

    monkeypatch.setattr(cli, "WebProvider", NumberedProvider)
    result = CliRunner().invoke(cli.main, ["read", "chapter-1", "--no-pager"])

    assert result.exit_code == 0
    assert result.output.count("第1章 开篇") == 1
    assert "第 1 章  第1章" not in result.output


def test_browse_can_advance_then_quit(monkeypatch) -> None:
    monkeypatch.setattr(cli, "WebProvider", FakeProvider)

    result = CliRunner().invoke(
        cli.main,
        ["browse", "book-1", "--chapter", "1", "--no-pager"],
        input="n\nq\n",
    )

    assert result.exit_code == 0
    assert "第 1 章" in result.output
    assert "第 2 章" in result.output


def test_domain_error_has_no_traceback(monkeypatch) -> None:
    class FailingProvider(FakeProvider):
        def search(self, query: str, offset: int = 0) -> SearchPage:
            raise NetworkError("网络不可用")

    monkeypatch.setattr(cli, "WebProvider", FailingProvider)
    result = CliRunner().invoke(cli.main, ["search", "书"])

    assert result.exit_code == 1
    assert "Error: 网络不可用" in result.output
    assert "Traceback" not in result.output
