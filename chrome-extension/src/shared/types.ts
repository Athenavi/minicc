// ── RPA 消息类型 ──

export type RPAMessageType = "command" | "result" | "event" | "ack";

export interface RPAMessage {
  type: RPAMessageType;
  id: string;
  method?: string;
  params?: Record<string, unknown>;
  result?: Record<string, unknown>;
  error?: RPAError;
  tabId?: number;
  ts: number;
}

export interface RPAError {
  code: number;
  message: string;
}

// ── 命令方法枚举 ──

export type RPAMethod =
  | "browser_navigate"
  | "browser_click"
  | "browser_type"
  | "browser_read"
  | "browser_screenshot"
  | "browser_scroll"
  | "browser_get_state"
  | "browser_tab_list"
  | "browser_tab_create"
  | "browser_tab_switch"
  | "browser_tab_close"
  | "browser_cookie_get"
  | "browser_cookie_set"
  | "browser_cookie_delete"
  | "browser_storage_get"
  | "browser_storage_set"
  | "browser_keypress";

// ── Tab 信息 ──

export interface RPATabInfo {
  id: number;
  url: string;
  title: string;
  active: boolean;
}

// ── 页面状态 ──

export interface PageState {
  url: string;
  title: string;
  scrollY: number;
  scrollHeight: number;
  viewportHeight: number;
  elements: PageElement[];
  forms: FormInfo[];
}

export interface PageElement {
  tag: string;
  selector: string;
  text: string;
  role: string;
  visible: boolean;
  rect: { x: number; y: number; width: number; height: number };
}

export interface FormInfo {
  selector: string;
  fields: FormField[];
}

export interface FormField {
  name: string;
  type: string;
  value: string;
  selector: string;
  label: string;
}

// ── Content Script → Service Worker 消息 ──

export type CSMessageType = "CS_EXECUTE" | "CS_COLLECT_STATE" | "CS_READY";

export interface CSMessage {
  type: CSMessageType;
  payload?: Record<string, unknown>;
}

export type SWMessageType = "SW_EXECUTE" | "SW_COLLECT_STATE" | "SW_RESULT";

export interface SWMessage {
  type: SWMessageType;
  payload?: Record<string, unknown>;
  error?: string;
}
