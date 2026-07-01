"""Playwright 浏览器管理器 — 管理浏览器实例和标签页。"""

from __future__ import annotations

import asyncio
import logging
from typing import Optional

logger = logging.getLogger("minicc.web.browser")


class BrowserManager:
    """Playwright 浏览器管理器。管理浏览器实例和标签页。

    每个会话独享一个 BrowserContext，互不干扰。
    支持多标签页、Cookie 管理、截图。
    """

    def __init__(self) -> None:
        self._browser = None
        self._context = None
        self._pages: dict[str, "Page"] = {}
        self._current_page_id: str | None = None
        self._page_counter = 0
        self._launched = False

    async def launch(self, headless: bool = True) -> bool:
        """启动浏览器。返回是否成功。"""
        try:
            from playwright.async_api import async_playwright
            self._pw = await async_playwright().start()
            self._browser = await self._pw.chromium.launch(
                headless=headless,
                args=[
                    "--no-sandbox",
                    "--disable-setuid-sandbox",
                    "--disable-dev-shm-usage",
                ],
            )
            self._context = await self._browser.new_context(
                viewport={"width": 1280, "height": 720},
                user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
            )
            self._launched = True
            logger.info("Browser launched (headless=%s)", headless)
            return True
        except Exception as exc:
            logger.warning("Browser launch failed: %s", exc)
            return False

    async def new_page(self, url: str | None = None) -> str:
        """创建新标签页，返回 page_id。"""
        if not self._context:
            raise RuntimeError("Browser not launched")
        page = await self._context.new_page()
        self._page_counter += 1
        page_id = f"page_{self._page_counter}"
        self._pages[page_id] = page
        self._current_page_id = page_id
        if url:
            await page.goto(url, wait_until="domcontentloaded", timeout=30000)
        return page_id

    async def navigate(self, page_id: str, url: str) -> bool:
        """导航到 URL。"""
        page = self._get_page(page_id)
        try:
            await page.goto(url, wait_until="domcontentloaded", timeout=30000)
            return True
        except Exception as exc:
            logger.warning("Navigate failed: %s", exc)
            return False

    async def click(self, page_id: str, selector: str) -> bool:
        """点击元素。支持 CSS 选择器。"""
        page = self._get_page(page_id)
        try:
            await page.click(selector, timeout=10000)
            return True
        except Exception as exc:
            logger.warning("Click failed: %s", exc)
            return False

    async def fill(self, page_id: str, selector: str, text: str) -> bool:
        """输入文本。"""
        page = self._get_page(page_id)
        try:
            await page.fill(selector, text, timeout=10000)
            return True
        except Exception as exc:
            logger.warning("Fill failed: %s", exc)
            return False

    async def select_option(self, page_id: str, selector: str, value: str) -> bool:
        """下拉选择。"""
        page = self._get_page(page_id)
        try:
            await page.select_option(selector, value, timeout=10000)
            return True
        except Exception as exc:
            logger.warning("Select failed: %s", exc)
            return False

    async def get_html(self, page_id: str, selector: str | None = None) -> str:
        """获取页面 HTML 内容。"""
        page = self._get_page(page_id)
        try:
            if selector:
                el = await page.query_selector(selector)
                if not el:
                    return f"Element not found: {selector}"
                return await el.inner_html()
            return await page.content()
        except Exception as exc:
            return f"Get HTML failed: {exc}"

    async def get_text(self, page_id: str, selector: str) -> str:
        """获取元素文本。"""
        page = self._get_page(page_id)
        try:
            el = await page.query_selector(selector)
            if not el:
                return f"Element not found: {selector}"
            return await el.inner_text()
        except Exception as exc:
            return f"Get text failed: {exc}"

    async def screenshot(self, page_id: str, full_page: bool = False) -> str:
        """截取页面截图，返回 base64。"""
        page = self._get_page(page_id)
        try:
            b64 = await page.screenshot(full_page=full_page, type="png")
            import base64
            return base64.b64encode(b64).decode()
        except Exception as exc:
            return f"Screenshot failed: {exc}"

    async def get_url(self, page_id: str) -> str:
        """获取当前 URL。"""
        page = self._get_page(page_id)
        return page.url

    async def get_title(self, page_id: str) -> str:
        """获取页面标题。"""
        page = self._get_page(page_id)
        return await page.title()

    async def wait_for_selector(self, page_id: str, selector: str, timeout: int = 15) -> bool:
        """等待元素出现。"""
        page = self._get_page(page_id)
        try:
            await page.wait_for_selector(selector, timeout=timeout * 1000)
            return True
        except Exception:
            return False

    async def list_pages(self) -> list[dict]:
        """列出所有标签页信息。"""
        result = []
        for pid, page in self._pages.items():
            try:
                result.append({
                    "id": pid,
                    "url": page.url,
                    "title": await page.title(),
                    "current": pid == self._current_page_id,
                })
            except Exception:
                result.append({"id": pid, "url": "(closed)", "title": "", "current": False})
        return result

    async def switch_page(self, page_id: str) -> bool:
        """切换到指定标签页。"""
        if page_id in self._pages:
            self._current_page_id = page_id
            return True
        return False

    async def close_page(self, page_id: str) -> None:
        """关闭标签页。"""
        page = self._pages.pop(page_id, None)
        if page:
            try:
                await page.close()
            except Exception:
                pass
        if self._current_page_id == page_id:
            self._current_page_id = next(iter(self._pages)) if self._pages else None

    async def close(self) -> None:
        """关闭浏览器。"""
        if self._browser:
            try:
                await self._browser.close()
            except Exception:
                pass
        if hasattr(self, "_pw"):
            try:
                await self._pw.stop()
            except Exception:
                pass
        self._launched = False
        self._pages.clear()
        self._current_page_id = None
        logger.info("Browser closed")

    @property
    def launched(self) -> bool:
        return self._launched

    @property
    def current_page_id(self) -> str | None:
        return self._current_page_id

    def _get_page(self, page_id: str):
        page = self._pages.get(page_id)
        if not page:
            raise ValueError(f"Page not found: {page_id}")
        return page


# 全局浏览器管理器实例
browser_manager = BrowserManager()
