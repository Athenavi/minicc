import type { RPAMessage } from "../shared/types";
import { logger } from "../shared/logger";
import {
  WS_RECONNECT_BASE_DELAY,
  WS_RECONNECT_MAX_DELAY,
  WS_RECONNECT_JITTER,
  STORAGE_KEYS,
} from "../shared/constants";

type MessageHandler = (msg: RPAMessage) => void;
type StatusHandler = (connected: boolean) => void;

export class WSClient {
  private ws: WebSocket | null = null;
  private url = "";
  private reconnectDelay = WS_RECONNECT_BASE_DELAY;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private messageHandlers: MessageHandler[] = [];
  private statusHandlers: StatusHandler[] = [];
  private stopped = false;

  onMessage(handler: MessageHandler): void {
    this.messageHandlers.push(handler);
  }

  onStatus(handler: StatusHandler): void {
    this.statusHandlers.push(handler);
  }

  async connect(): Promise<void> {
    const stored = await chrome.storage.local.get([
      STORAGE_KEYS.WS_URL,
      STORAGE_KEYS.CLIENT_ID,
    ]);
    this.url = stored[STORAGE_KEYS.WS_URL] || "ws://localhost:8080/ws/rpa";
    const clientId =
      stored[STORAGE_KEYS.CLIENT_ID] || this.generateClientId();

    await chrome.storage.local.set({ [STORAGE_KEYS.CLIENT_ID]: clientId });
    this.stopped = false;
    this.doConnect(clientId);
  }

  disconnect(): void {
    this.stopped = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.notifyStatus(false);
  }

  send(msg: RPAMessage): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      logger.warn("Cannot send, WebSocket not open");
      return;
    }
    this.ws.send(JSON.stringify(msg));
  }

  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  private doConnect(clientId: string): void {
    if (this.stopped) return;

    const wsUrl = `${this.url}?client_id=${encodeURIComponent(clientId)}`;
    logger.info("Connecting to", wsUrl);

    try {
      this.ws = new WebSocket(wsUrl);
    } catch (err) {
      logger.error("WebSocket constructor failed:", err);
      this.scheduleReconnect(clientId);
      return;
    }

    this.ws.onopen = () => {
      logger.info("WebSocket connected");
      this.reconnectDelay = WS_RECONNECT_BASE_DELAY;
      this.notifyStatus(true);
      this.sendInitEvent();
    };

    this.ws.onmessage = (event: MessageEvent) => {
      try {
        const msg: RPAMessage = JSON.parse(event.data as string);
        for (const handler of this.messageHandlers) {
          handler(msg);
        }
      } catch (err) {
        logger.error("Failed to parse message:", err);
      }
    };

    this.ws.onclose = (event: CloseEvent) => {
      logger.info("WebSocket closed:", event.code, event.reason);
      this.notifyStatus(false);
      if (!this.stopped) {
        this.scheduleReconnect(clientId);
      }
    };

    this.ws.onerror = (event: Event) => {
      logger.error("WebSocket error:", event);
    };
  }

  private scheduleReconnect(clientId: string): void {
    const jitter = Math.random() * WS_RECONNECT_JITTER;
    const delay = Math.min(
      this.reconnectDelay + jitter,
      WS_RECONNECT_MAX_DELAY
    );
    logger.info(`Reconnecting in ${Math.round(delay)}ms...`);
    this.reconnectTimer = setTimeout(() => {
      this.doConnect(clientId);
    }, delay);
    this.reconnectDelay = Math.min(
      this.reconnectDelay * 2,
      WS_RECONNECT_MAX_DELAY
    );
  }

  private notifyStatus(connected: boolean): void {
    chrome.storage.local.set({
      [STORAGE_KEYS.CONNECTION_STATUS]: connected
        ? "connected"
        : "disconnected",
    });
    for (const handler of this.statusHandlers) {
      handler(connected);
    }
  }

  private async sendInitEvent(): Promise<void> {
    try {
      const tabs = await chrome.tabs.query({});
      const tabInfos = tabs.map((t) => ({
        id: t.id ?? 0,
        url: t.url ?? "",
        title: t.title ?? "",
        active: t.active,
      }));
      this.send({
        type: "event",
        id: `evt_${Date.now()}`,
        method: "init",
        params: { tabs: tabInfos },
        ts: Date.now(),
      });
    } catch (err) {
      logger.error("Failed to send init event:", err);
    }
  }

  private generateClientId(): string {
    const arr = new Uint8Array(16);
    crypto.getRandomValues(arr);
    return Array.from(arr, (b) => b.toString(16).padStart(2, "0")).join("");
  }
}
