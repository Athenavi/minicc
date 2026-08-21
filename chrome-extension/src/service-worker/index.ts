import { logger, setLogLevel, LogLevel } from "../shared/logger";
import { WSClient } from "./ws-client";
import { CommandRouter } from "./command-router";
import type { RPAMessage } from "../shared/types";

setLogLevel(LogLevel.DEBUG);

const wsClient = new WSClient();
const commandRouter = new CommandRouter(wsClient);

wsClient.onMessage((msg: RPAMessage) => {
  switch (msg.type) {
    case "command":
      commandRouter.handleCommand(msg);
      break;
    case "ack":
      logger.info("Received ack:", msg.id, msg.result);
      break;
    default:
      logger.debug("Unhandled message type:", msg.type);
  }
});

wsClient.onStatus((connected) => {
  logger.info("Connection status:", connected ? "connected" : "disconnected");
});

chrome.runtime.onInstalled.addListener((details) => {
  logger.info("Extension installed:", details.reason);
  chrome.storage.local.get("rpa_ws_url", (data) => {
    if (!data.rpa_ws_url) {
      chrome.storage.local.set({
        rpa_ws_url: "ws://localhost:8080/ws/rpa",
      });
    }
  });
});

wsClient.connect();

chrome.runtime.onStartup.addListener(() => {
  logger.info("Service worker starting up");
  wsClient.connect();
});

// Tab event forwarding
chrome.tabs.onCreated.addListener((tab) => {
  if (wsClient.isConnected()) {
    wsClient.send({
      type: "event",
      id: `evt_${Date.now()}`,
      method: "tab_created",
      tabId: tab.id,
      params: { url: tab.url, title: tab.title },
      ts: Date.now(),
    });
  }
});

chrome.tabs.onRemoved.addListener((tabId) => {
  if (wsClient.isConnected()) {
    wsClient.send({
      type: "event",
      id: `evt_${Date.now()}`,
      method: "tab_closed",
      tabId,
      ts: Date.now(),
    });
  }
});

chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
  if (wsClient.isConnected() && changeInfo.status === "complete") {
    wsClient.send({
      type: "event",
      id: `evt_${Date.now()}`,
      method: "tab_updated",
      tabId,
      params: { url: changeInfo.url, title: changeInfo.title },
      ts: Date.now(),
    });
  }
});

// Message from popup
chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message.type === "GET_STATUS") {
    sendResponse({ connected: wsClient.isConnected() });
    return true;
  }
  if (message.type === "RECONNECT") {
    wsClient.disconnect();
    wsClient.connect();
    sendResponse({ ok: true });
    return true;
  }
  if (message.type === "SET_WS_URL") {
    chrome.storage.local.set({ rpa_ws_url: message.url }, () => {
      wsClient.disconnect();
      wsClient.connect();
      sendResponse({ ok: true });
    });
    return true;
  }
  return false;
});

logger.info("Service worker initialized");
