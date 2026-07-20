// ── WebSocket ──

export const WS_RECONNECT_BASE_DELAY = 1000; // ms
export const WS_RECONNECT_MAX_DELAY = 30000; // ms
export const WS_RECONNECT_JITTER = 500; // ms
export const WS_HEARTBEAT_INTERVAL = 25_000; // ms

// ── Command ──

export const COMMAND_TIMEOUT = 30_000; // ms
export const DEFAULT_SCROLL_AMOUNT = 500; // px

// ── Selector ──

export const SELECTOR_STRATEGIES = ["css", "xpath", "text", "aria"] as const;
export type SelectorStrategy = (typeof SELECTOR_STRATEGIES)[number];

// ── Error Codes ──

export const RPAErrorCode = {
  ELEMENT_NOT_FOUND: 1001,
  ELEMENT_NOT_VISIBLE: 1002,
  ELEMENT_NOT_INTERACTABLE: 1003,
  NAVIGATION_TIMEOUT: 2001,
  NAVIGATION_ERROR: 2002,
  TAB_NOT_FOUND: 3001,
  TAB_CREATE_FAILED: 3002,
  PERMISSION_DENIED: 4001,
  COMMAND_TIMEOUT: 5001,
  UNKNOWN_ERROR: 9999,
} as const;

// ── Storage Keys ──

export const STORAGE_KEYS = {
  WS_URL: "rpa_ws_url",
  CONNECTION_STATUS: "rpa_connection_status",
  USER_TOKEN: "rpa_user_token",
  CLIENT_ID: "rpa_client_id",
} as const;
