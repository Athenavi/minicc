import type { RPAMessage, RPAError, SWMessage } from "../shared/types";
import { RPAErrorCode, COMMAND_TIMEOUT } from "../shared/constants";
import { logger } from "../shared/logger";
import type { WSClient } from "./ws-client";
import { TabManager } from "./tab-manager";

export class CommandRouter {
  private tabManager: TabManager;

  constructor(private wsClient: WSClient) {
    this.tabManager = new TabManager();
  }

  async handleCommand(msg: RPAMessage): Promise<void> {
    const { id, method, params, tabId } = msg;
    if (!id || !method) {
      logger.warn("Invalid command message:", msg);
      return;
    }

    logger.info(`Executing command: ${method}`, params);

    try {
      let result: Record<string, unknown>;

      if (this.isTabCommand(method)) {
        result = await this.handleTabCommand(method, params);
      } else {
        result = await this.forwardToContentScript(method, params, tabId);
      }

      this.wsClient.send({
        type: "result",
        id,
        result,
        ts: Date.now(),
      });
    } catch (err) {
      const error: RPAError = {
        code: RPAErrorCode.UNKNOWN_ERROR,
        message: err instanceof Error ? err.message : String(err),
      };
      if (error.message.includes("not found")) {
        error.code = RPAErrorCode.ELEMENT_NOT_FOUND;
      } else if (error.message.includes("timeout")) {
        error.code = RPAErrorCode.COMMAND_TIMEOUT;
      } else if (error.message.includes("Tab")) {
        error.code = RPAErrorCode.TAB_NOT_FOUND;
      }

      this.wsClient.send({
        type: "result",
        id,
        error,
        ts: Date.now(),
      });
    }
  }

  private isTabCommand(method: string): boolean {
    return [
      "browser_tab_list",
      "browser_tab_create",
      "browser_tab_switch",
      "browser_tab_close",
      "browser_navigate",
      "browser_screenshot",
    ].includes(method);
  }

  private async handleTabCommand(
    method: string,
    params?: Record<string, unknown>
  ): Promise<Record<string, unknown>> {
    switch (method) {
      case "browser_tab_list": {
        const tabs = await this.tabManager.listTabs();
        return { tabs };
      }
      case "browser_tab_create": {
        const url = params?.url as string | undefined;
        const tab = await this.tabManager.createTab(url);
        return { tab };
      }
      case "browser_tab_switch": {
        const tabId = params?.tabId as number;
        if (!tabId) throw new Error("tabId is required");
        const tab = await this.tabManager.switchTab(tabId);
        return { tab };
      }
      case "browser_tab_close": {
        const tabId = params?.tabId as number;
        if (!tabId) throw new Error("tabId is required");
        await this.tabManager.closeTab(tabId);
        return { closed: true };
      }
      case "browser_navigate": {
        const url = params?.url as string;
        if (!url) throw new Error("url is required");
        // 在新标签页中打开 URL 并等待加载完成，确保 content script 就绪
        const tab = await this.tabManager.createTabAndWait(url);
        return { tab, navigated_to: url };
      }
      case "browser_screenshot": {
        const tabId = params?.tabId as number | undefined;
        const fullPage = params?.fullPage as boolean | undefined;
        const result = await this.tabManager.captureScreenshot(tabId, fullPage);
        return result;
      }
      default:
        throw new Error(`Unknown tab command: ${method}`);
    }
  }

  private async forwardToContentScript(
    method: string,
    params?: Record<string, unknown>,
    tabId?: number
  ): Promise<Record<string, unknown>> {
    const targetTab = await this.tabManager.getTabForCommand(tabId);
    if (!targetTab.id) throw new Error("Target tab has no ID");

    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(
          new Error(
            `Command ${method} timed out after ${COMMAND_TIMEOUT}ms`
          )
        );
      }, COMMAND_TIMEOUT);

      chrome.tabs.sendMessage(
        targetTab.id!,
        { type: "SW_EXECUTE", payload: { method, params } } as SWMessage,
        (response) => {
          clearTimeout(timeout);
          if (chrome.runtime.lastError) {
            reject(
              new Error(
                chrome.runtime.lastError.message || "Content script error"
              )
            );
            return;
          }
          if (response?.error) {
            const errMsg = typeof response.error === 'string' ? response.error : (response.error as any).message || JSON.stringify(response.error);
            reject(new Error(errMsg));
            return;
          }
          resolve(response?.data || {});
        }
      );
    });
  }
}
