package fanqie

import (
	"strings"
	"testing"
)

func TestExtractInitialStateAllowsBracesInsideStrings(t *testing.T) {
	page := `<script>window.__INITIAL_STATE__ = {"text":"a } brace","page":{"id":1}};</script>`
	state, err := extractInitialState(page)
	if err != nil {
		t.Fatal(err)
	}
	if state["text"] != "a } brace" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestExtractInitialStateReportsMissingMarker(t *testing.T) {
	_, err := extractInitialState("<html></html>")
	if err == nil || !strings.Contains(err.Error(), "__INITIAL_STATE__") {
		t.Fatalf("expected initial state error, got %v", err)
	}
}

func TestHTMLToText(t *testing.T) {
	got := htmlToText("<div><p>第一段&nbsp;&amp;</p><p>第二行<br>续行</p></div>")
	want := "第一段\u00a0&\n\n第二行\n续行"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDecryptKnownPrivateUseRune(t *testing.T) {
	got := decrypt("甲\ue3e8乙")
	if got != "甲D乙" {
		t.Fatalf("got %q", got)
	}
	if decrypt("\uf8ff") != "\uf8ff" {
		t.Fatal("unknown private-use character should remain untouched")
	}
}

func TestCategoryFallback(t *testing.T) {
	page := map[string]any{"categoryV2": `[{"Name":"玄幻"},{"Name":"穿越"}]`}
	if got := category(page); got != "玄幻 / 穿越" {
		t.Fatalf("got %q", got)
	}
}
