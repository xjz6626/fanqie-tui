import pytest

from fanqie_tui.errors import ParseError
from fanqie_tui.parsing import as_bool, extract_initial_state, html_to_text


def test_extract_initial_state_handles_braces_inside_strings() -> None:
    page = '<script>window.__INITIAL_STATE__ = {"text":"a } brace", "page":{"id":1}};</script>'

    assert extract_initial_state(page) == {"text": "a } brace", "page": {"id": 1}}


def test_extract_initial_state_reports_missing_data() -> None:
    with pytest.raises(ParseError, match="__INITIAL_STATE__"):
        extract_initial_state("<html></html>")


def test_html_to_text_preserves_paragraphs_and_decodes_entities() -> None:
    source = "<div><p>第一段&nbsp;&amp;</p><p>第二行<br>续行</p></div>"

    assert html_to_text(source) == "第一段\xa0&\n\n第二行\n续行"


@pytest.mark.parametrize(
    ("value", "expected"),
    [(False, False), (True, True), (0, False), (1, True), ("0", False), ("1", True), ("false", False)],
)
def test_as_bool(value: object, expected: bool) -> None:
    assert as_bool(value) is expected
