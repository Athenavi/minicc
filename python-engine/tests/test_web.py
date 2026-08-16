"""Tests for web tools — web_search 与 HTML→markdown 渲染。"""
from __future__ import annotations

import pytest

from app.tools.web import (
    DuckDuckGoProvider,
    WebProvider,
    html_to_markdown,
    set_web_provider,
    web_fetch,
    web_search,
)


class FakeProvider(WebProvider):
    def __init__(self, results=None, fetch_result=None):
        self._results = results or []
        self._fetch = fetch_result or {}

    async def search(self, query, max_results):
        return self._results

    async def fetch(self, url, max_chars):
        return self._fetch


class TestHtmlToMarkdown:
    def test_headings_paragraphs(self):
        md = html_to_markdown("<h1>Title</h1><p>Hello <strong>world</strong>.</p>")
        assert "# Title" in md
        assert "Hello" in md and "world" in md

    def test_list_and_code(self):
        md = html_to_markdown("<ul><li>a</li><li>b</li></ul><pre>code line</pre>")
        assert "- a" in md and "- b" in md
        assert "```" in md and "code line" in md

    def test_links_and_tables(self):
        md = html_to_markdown('<p><a href="https://x.com">X</a></p><table><tr><td>c1</td><td>c2</td></tr></table>')
        assert "[X](https://x.com)" in md
        assert "| c1 | c2 |" in md

    def test_strips_script_style(self):
        md = html_to_markdown("<script>bad()</script><style>.x{}</style><p>good</p>")
        assert "bad" not in md and "good" in md


class TestWebTools:
    @pytest.mark.asyncio
    async def test_web_search_returns_results(self):
        set_web_provider(FakeProvider(results=[
            {"title": "T", "url": "https://e.com", "snippet": "s"},
        ]))
        out = await web_search("python")
        assert out["count"] == 1
        assert out["results"][0]["title"] == "T"

    @pytest.mark.asyncio
    async def test_web_search_empty_query(self):
        out = await web_search("  ")
        assert "error" in out

    @pytest.mark.asyncio
    async def test_web_fetch_returns_content(self):
        set_web_provider(FakeProvider(fetch_result={
            "url": "https://e.com", "status_code": 200, "content_type": "text/html", "content": "ok",
        }))
        out = await web_fetch("https://e.com")
        assert out["content"] == "ok"

    @pytest.mark.asyncio
    async def test_web_search_provider_error(self):
        class Broken(FakeProvider):
            async def search(self, query, max_results):
                raise RuntimeError("provider down")
        set_web_provider(Broken())
        out = await web_search("q")
        assert "error" in out and "unavailable" in out["error"]

    def test_ddg_redirect_decode(self):
        url = DuckDuckGoProvider._decode_redirect("//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage")
        assert url == "https://example.com/page"
