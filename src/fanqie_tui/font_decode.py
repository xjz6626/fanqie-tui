"""Decode the private-use characters used by Fanqie's web reader."""

from __future__ import annotations

import io
import re
from collections.abc import Callable

# Glyph order originally documented by the fanqienovel-decryptor project.
# Index zero corresponds to glyph id 58344. Question marks are unknown glyphs
# and intentionally remain untouched.
GLYPH_CHARSET = (
    "D在主特家军然表场4要只v和?6别还g现儿岁??此象月3出战工相o男直失世F都平文什VO将真T那当?"
    "会立些u是十张学气大爱两命全后东性通被1它乐接而感车山公了常以何可话先pi叫轻M士w着变尔快"
    "l个说少色里安花远7难师放t报认面道S?克地度I好机U民写把万同水新没书电吃像斯5为y白几日教"
    "看但第加候作上拉住有法r事应位利你声身国问马女他Y比父xAHNsX边美对所金活回意到z从j知又内"
    "因点Q三定8Rb正或夫向德听更?得告并本q过记L让打f人就者去原满体做经K走如孩cG给使物?最笑部?"
    "员等受k行一条果动光门头见往自解成处天能于名其发总母的死手入路进心来h时力多开已许d至由很"
    "界n小与Z想代么分生口再妈望次西风种带J?实情才这?E我神格长觉间年眼无不亲关结0友信下却重己"
    "老2音字m呢明之前高PB目太e9起稜她也W用方子英每理便四数期中C外样a海们任"
)
_BASE_GLYPH_ID = 58344
_FONT_URL = re.compile(r"url\([\"']?(https?://[^)\"']+?\.woff2(?:\?[^)\"']*)?)[\"']?\)")
_GLYPH_NUMBER = re.compile(r"(\d+)$")


def _known_character(index: int) -> str | None:
    if 0 <= index < len(GLYPH_CHARSET):
        value = GLYPH_CHARSET[index]
        return None if value == "?" else value
    return None


def fallback_mapping() -> dict[str, str]:
    """Return the stable positional mapping used when no font can be parsed."""

    return {
        chr(_BASE_GLYPH_ID + index): value
        for index in range(len(GLYPH_CHARSET))
        if (value := _known_character(index)) is not None
    }


def find_font_url(page_html: str, state: dict[str, object] | None = None) -> str | None:
    sources: list[str] = []
    if state:
        common = state.get("common")
        if isinstance(common, dict) and isinstance(common.get("css"), str):
            sources.append(common["css"])
    sources.append(page_html)
    for source in sources:
        match = _FONT_URL.search(source)
        if match:
            return match.group(1)
    return None


def build_mapping(
    page_html: str,
    state: dict[str, object],
    fetch_bytes: Callable[[str], bytes],
) -> dict[str, str]:
    """Build a PUA-to-text mapping from the page's WOFF2 cmap.

    Failure is deliberately non-fatal: the positional mapping covers the
    currently observed web font and lets metadata/catalog commands keep working
    even when the optional font dependency or font CDN is unavailable.
    """

    mapping = fallback_mapping()
    font_url = find_font_url(page_html, state)
    if not font_url:
        return mapping
    try:
        from fontTools.ttLib import TTFont

        font = TTFont(io.BytesIO(fetch_bytes(font_url)))
        cmap = font.getBestCmap() or {}
    except Exception:
        return mapping

    dynamic: dict[str, str] = {}
    for codepoint, glyph_name in cmap.items():
        match = _GLYPH_NUMBER.search(glyph_name)
        if not match:
            continue
        character = _known_character(int(match.group(1)) - _BASE_GLYPH_ID)
        if character is not None:
            dynamic[chr(codepoint)] = character
    if dynamic:
        mapping.update(dynamic)
    return mapping


def decrypt(text: str, mapping: dict[str, str] | None = None) -> str:
    active = mapping or fallback_mapping()
    return "".join(active.get(character, character) for character in text)
