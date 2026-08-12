"""Command-line interface and interactive terminal reader."""

from __future__ import annotations

import functools
import re
import sys
from collections.abc import Callable
from typing import Any, TypeVar

import click

from . import __version__
from .errors import FanqieError
from .models import Book, Chapter, ChapterContent
from .web_provider import WebProvider

_COMMAND = TypeVar("_COMMAND", bound=Callable[..., Any])
_NUMBERED_CHAPTER = re.compile(r"^第\s*0*(\d+)\s*章")


def _handle_errors(function: _COMMAND) -> _COMMAND:
    """Turn expected provider failures into concise command-line errors."""

    @functools.wraps(function)
    def wrapped(*args: Any, **kwargs: Any) -> Any:
        try:
            return function(*args, **kwargs)
        except FanqieError as exc:
            raise click.ClickException(str(exc)) from exc

    return wrapped  # type: ignore[return-value]


def _provider(context: click.Context) -> WebProvider:
    return WebProvider(timeout=context.ensure_object(dict)["timeout"])


def _compact_number(value: int) -> str:
    if value >= 100_000_000:
        return f"{value / 100_000_000:.1f}亿".replace(".0亿", "亿")
    if value >= 10_000:
        return f"{value / 10_000:.1f}万".replace(".0万", "万")
    return str(value)


def _book_summary(book: Book, *, include_abstract: bool = True) -> str:
    details = [value for value in (book.author, book.category, book.status) if value]
    if book.word_count:
        details.append(f"{_compact_number(book.word_count)}字")
    if book.chapter_count:
        details.append(f"{book.chapter_count}章")
    if book.score:
        details.append(f"{book.score:g}分")
    lines = [book.title, f"书籍 ID：{book.id}"]
    if details:
        lines.append(" · ".join(details))
    if include_abstract and book.abstract:
        lines.extend(("", book.abstract))
    return "\n".join(lines)


def _chapter_label(chapter: Chapter) -> str:
    flags: list[str] = []
    if chapter.locked:
        flags.append("已锁定")
    elif chapter.need_pay:
        flags.append("需付费")
    suffix = f" [{', '.join(flags)}]" if flags else ""
    volume = f"{chapter.volume} · " if chapter.volume else ""
    return f"{chapter.order:>5}  {volume}{chapter.title}{suffix}"


def _chapter_text(chapter: ChapterContent) -> str:
    heading = chapter.title
    title_match = _NUMBERED_CHAPTER.match(chapter.title)
    title_has_order = title_match and int(title_match.group(1)) == chapter.order
    if chapter.order and not title_has_order:
        heading = f"第 {chapter.order} 章  {heading}"
    return f"{heading}\n{'─' * min(max(len(heading), 12), 48)}\n\n{chapter.content}\n"


def _show_text(text: str, *, pager: bool) -> None:
    if pager:
        click.echo_via_pager(text)
    else:
        click.echo(text)


@click.group(invoke_without_command=True, context_settings={"help_option_names": ["-h", "--help"]})
@click.option(
    "--timeout",
    type=click.FloatRange(min=1.0),
    default=20.0,
    show_default=True,
    help="网络请求超时秒数。",
)
@click.version_option(__version__, prog_name="fanqie")
@click.pass_context
def main(context: click.Context, timeout: float) -> None:
    """在终端中搜索和阅读番茄小说的公开网页内容。"""

    context.ensure_object(dict)["timeout"] = timeout
    if context.invoked_subcommand is None:
        click.echo(context.get_help())


@main.command("search")
@click.argument("query")
@click.option("--offset", type=click.IntRange(min=0), default=0, show_default=True)
@click.pass_context
@_handle_errors
def search_command(context: click.Context, query: str, offset: int) -> None:
    """按书名或作者搜索。"""

    with _provider(context) as provider:
        page = provider.search(query, offset)
    if not page.books:
        click.echo("没有找到匹配的书籍。")
        return
    for index, book in enumerate(page.books, start=1):
        click.echo(f"{index:>2}. {_book_summary(book, include_abstract=False)}")
        if index != len(page.books):
            click.echo()
    if page.has_more:
        click.echo(f"\n还有更多结果：fanqie search {query!r} --offset {page.next_offset}")


@main.command("info")
@click.argument("book_id")
@click.pass_context
@_handle_errors
def info_command(context: click.Context, book_id: str) -> None:
    """显示 BOOK_ID 对应的书籍信息。"""

    with _provider(context) as provider:
        book = provider.get_book(book_id)
    click.echo(_book_summary(book))


@main.command("catalog")
@click.argument("book_id")
@click.option("--start", type=click.IntRange(min=1), default=1, show_default=True, help="从第几项开始显示。")
@click.option("--limit", type=click.IntRange(min=1), default=50, show_default=True, help="最多显示多少项。")
@click.pass_context
@_handle_errors
def catalog_command(context: click.Context, book_id: str, start: int, limit: int) -> None:
    """显示 BOOK_ID 的章节目录。"""

    with _provider(context) as provider:
        chapters = provider.get_directory(book_id)
    selected = chapters[start - 1 : start - 1 + limit]
    if not selected:
        raise click.ClickException(f"起始项 {start} 超出目录范围（共 {len(chapters)} 章）")
    for chapter in selected:
        click.echo(f"{_chapter_label(chapter)}  ({chapter.id})")
    shown_through = start + len(selected) - 1
    if shown_through < len(chapters):
        click.echo(f"\n已显示 {start}–{shown_through}，共 {len(chapters)} 章；使用 --start {shown_through + 1} 继续。")


@main.command("read")
@click.argument("item_id")
@click.option("--pager/--no-pager", default=None, help="是否通过终端分页器显示正文。")
@click.pass_context
@_handle_errors
def read_command(context: click.Context, item_id: str, pager: bool | None) -> None:
    """阅读 ITEM_ID 对应的公开章节。"""

    with _provider(context) as provider:
        chapter = provider.get_chapter(item_id)
    _show_text(_chapter_text(chapter), pager=sys.stdout.isatty() if pager is None else pager)


@main.command("browse")
@click.argument("book_id")
@click.option("--chapter", type=click.IntRange(min=1), help="从目录中的第几项开始。")
@click.option("--pager/--no-pager", default=True, show_default=True, help="是否通过终端分页器显示正文。")
@click.pass_context
@_handle_errors
def browse_command(context: click.Context, book_id: str, chapter: int | None, pager: bool) -> None:
    """进入 BOOK_ID 的交互式连续阅读模式。"""

    with _provider(context) as provider:
        book = provider.get_book(book_id)
        chapters = provider.get_directory(book_id)
        click.echo(_book_summary(book, include_abstract=False))
        click.echo(f"目录共 {len(chapters)} 章。输入章节序号后可用 n/p/g/q 导航。\n")
        if chapter is None:
            chapter = click.prompt("从第几章开始", type=click.IntRange(1, len(chapters)), default=1)
        if chapter > len(chapters):
            raise click.ClickException(f"章节序号 {chapter} 超出目录范围（共 {len(chapters)} 章）")

        index = chapter - 1
        while True:
            item = chapters[index]
            if item.locked or item.need_pay:
                click.echo(f"\n{_chapter_label(item)}，无法从公开网页读取。")
            else:
                content = provider.get_chapter(item.id)
                _show_text(_chapter_text(content), pager=pager)

            action = click.prompt(
                "[n/回车] 下一章  [p] 上一章  [g] 跳转  [q] 退出",
                default="n",
                show_default=False,
            ).strip().lower()
            if action in {"q", "quit", "退出"}:
                break
            if action in {"g", "goto", "跳转"}:
                index = click.prompt("章节序号", type=click.IntRange(1, len(chapters))) - 1
                continue
            if action in {"p", "prev", "上一章"}:
                if index == 0:
                    click.echo("已经是第一章。")
                else:
                    index -= 1
                continue
            if action in {"", "n", "next", "下一章"}:
                if index == len(chapters) - 1:
                    click.echo("已经是最后一章。")
                else:
                    index += 1
                continue
            click.echo("无法识别该指令，请输入 n、p、g 或 q。")


if __name__ == "__main__":  # pragma: no cover
    main()
