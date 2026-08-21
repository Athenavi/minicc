import { executeAction } from "./dom-actions";
import { collectPageState } from "./state-collector";
import { logger } from "../shared/logger";
import type { SWMessage } from "../shared/types";

logger.info("Content script loaded on", window.location.href);

chrome.runtime.onMessage.addListener(
  (
    message: SWMessage,
    _sender: chrome.runtime.MessageSender,
    sendResponse: (response?: Record<string, unknown>) => void
  ) => {
    if (message.type === "SW_EXECUTE") {
      const { method, params } = message.payload as {
        method: string;
        params?: Record<string, unknown>;
      };
      logger.info(`Content script executing: ${method}`, params);

      try {
        const result = executeAction(method, params);
        sendResponse(result as unknown as Record<string, unknown>);
      } catch (err) {
        logger.error("Action execution error:", err);
        sendResponse({
          success: false,
          error: {
            code: 9999,
            message: err instanceof Error ? err.message : String(err),
          },
        });
      }
      return true;
    }

    if (message.type === "SW_COLLECT_STATE") {
      try {
        const state = collectPageState();
        sendResponse({ success: true, payload: state });
      } catch (err) {
        logger.error("State collection error:", err);
        sendResponse({
          success: false,
          error: err instanceof Error ? err.message : String(err),
        });
      }
      return true;
    }

    return false;
  }
);

chrome.runtime.sendMessage({ type: "CS_READY" });
