import type { RPATabInfo } from "../shared/types";
import { logger } from "../shared/logger";

export class TabManager {
  async listTabs(): Promise<RPATabInfo[]> {
    const tabs = await chrome.tabs.query({});
    return tabs.map((t) => ({
      id: t.id ?? 0,
      url: t.url ?? "",
      title: t.title ?? "",
      active: t.active,
    }));
  }

  async createTab(url?: string): Promise<RPATabInfo> {
    const tab = await chrome.tabs.create({ url: url || undefined });
    return {
      id: tab.id ?? 0,
      url: tab.url ?? "",
      title: tab.title ?? "",
      active: tab.active,
    };
  }

  /**
   * 创建新标签页并等待页面加载完成（content script 就绪）。
   * 用于 browser_navigate，确保后续命令能正常转发到 content script。
   */
  async createTabAndWait(url: string, timeoutMs = 15000): Promise<RPATabInfo> {
    const tab = await chrome.tabs.create({ url });
    const tabId = tab.id!;
    logger.info(`Tab ${tabId} created, waiting for load...`);

    // 等待 tab status 变为 "complete"
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => {
        chrome.tabs.onUpdated.removeListener(listener);
        reject(new Error(`Tab ${tabId} load timed out after ${timeoutMs}ms`));
      }, timeoutMs);

      const listener = (id: number, changeInfo: chrome.tabs.TabChangeInfo) => {
        if (id === tabId && changeInfo.status === "complete") {
          clearTimeout(timer);
          chrome.tabs.onUpdated.removeListener(listener);
          resolve();
        }
      };
      chrome.tabs.onUpdated.addListener(listener);

      // 检查是否已经加载完成（极快加载的情况）
      chrome.tabs.get(tabId).then((t) => {
        if (t.status === "complete") {
          clearTimeout(timer);
          chrome.tabs.onUpdated.removeListener(listener);
          resolve();
        }
      }).catch(() => { /* tab may not exist yet, ignore */ });
    });

    logger.info(`Tab ${tabId} loaded`);
    const loadedTab = await chrome.tabs.get(tabId);
    return {
      id: loadedTab.id ?? 0,
      url: loadedTab.url ?? "",
      title: loadedTab.title ?? "",
      active: loadedTab.active,
    };
  }

  async switchTab(tabId: number): Promise<RPATabInfo> {
    await chrome.tabs.update(tabId, { active: true });
    const tab = await chrome.tabs.get(tabId);
    if (tab.windowId !== undefined) {
      await chrome.windows.update(tab.windowId, { focused: true });
    }
    return {
      id: tab.id ?? 0,
      url: tab.url ?? "",
      title: tab.title ?? "",
      active: tab.active,
    };
  }

  async closeTab(tabId: number): Promise<void> {
    await chrome.tabs.remove(tabId);
  }

  async getActiveTab(): Promise<chrome.tabs.Tab | null> {
    const [tab] = await chrome.tabs.query({
      active: true,
      currentWindow: true,
    });
    return tab ?? null;
  }

  async getTabForCommand(tabId?: number): Promise<chrome.tabs.Tab> {
    if (tabId) {
      try {
        return await chrome.tabs.get(tabId);
      } catch {
        throw new Error(`Tab ${tabId} not found`);
      }
    }
    const tab = await this.getActiveTab();
    if (!tab) throw new Error("No active tab found");
    return tab;
  }

  /**
   * 使用 chrome.tabs.captureVisibleTab() 截取当前可见区域的截图。
   * 返回 base64 data URL、页面 URL 和标题。
   */
  async captureScreenshot(
    tabId?: number,
    fullPage?: boolean
  ): Promise<{ screenshot: string; url: string; title: string }> {
    const tab = tabId
      ? await chrome.tabs.get(tabId).catch(() => null)
      : await this.getActiveTab();
    if (!tab) throw new Error("No target tab for screenshot");
    if (!tab.windowId) throw new Error("Tab has no windowId");

    // 确保目标 tab 是活跃的（captureVisibleTab 只截取活跃 tab）
    if (!tab.active) {
      await chrome.tabs.update(tab.id!, { active: true });
      if (tab.windowId !== undefined) {
        await chrome.windows.update(tab.windowId, { focused: true });
      }
      // 等一小段时间让 tab 切换完成
      await new Promise((r) => setTimeout(r, 200));
    }

    const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, {
      format: "png",
    });

    logger.info(
      `Screenshot captured for tab ${tab.id}, data length: ${dataUrl.length}`
    );

    return {
      screenshot: dataUrl,
      url: tab.url ?? "",
      title: tab.title ?? "",
    };
  }
}
