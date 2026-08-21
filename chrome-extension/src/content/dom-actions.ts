import { resolveSelector, isVisible } from "./selector";
import { logger } from "../shared/logger";
import { RPAErrorCode } from "../shared/constants";

export interface ActionResult {
  success: boolean;
  data?: Record<string, unknown>;
  error?: { code: number; message: string };
}

export function executeAction(
  method: string,
  params?: Record<string, unknown>
): ActionResult {
  switch (method) {
    case "browser_click":
      return actionClick(params);
    case "browser_type":
      return actionType(params);
    case "browser_read":
      return actionRead(params);
    case "browser_scroll":
      return actionScroll(params);
    case "browser_get_state":
      return actionGetState();
    case "browser_screenshot":
      return actionScreenshot(params);
    case "browser_navigate":
      return actionNavigate(params);
    case "browser_keypress":
      return actionKeypress(params);
    default:
      return {
        success: false,
        error: {
          code: RPAErrorCode.UNKNOWN_ERROR,
          message: `Unknown action: ${method}`,
        },
      };
  }
}

function actionClick(params?: Record<string, unknown>): ActionResult {
  const selector = params?.selector as string;
  if (!selector) {
    return {
      success: false,
      error: {
        code: RPAErrorCode.ELEMENT_NOT_FOUND,
        message: "selector is required",
      },
    };
  }

  const el = resolveSelector(selector);
  if (!el) {
    return {
      success: false,
      error: {
        code: RPAErrorCode.ELEMENT_NOT_FOUND,
        message: `Element not found: ${selector}`,
      },
    };
  }

  if (!isVisible(el)) {
    el.scrollIntoView({ behavior: "smooth", block: "center" });
  }

  (el as HTMLElement).click();
  return { success: true, data: { clicked: selector } };
}

function actionType(params?: Record<string, unknown>): ActionResult {
  const selector = params?.selector as string;
  const text = params?.text as string;
  if (!selector || text === undefined) {
    return {
      success: false,
      error: {
        code: RPAErrorCode.ELEMENT_NOT_FOUND,
        message: "selector and text are required",
      },
    };
  }

  const el = resolveSelector(selector) as HTMLInputElement | null;
  if (!el) {
    return {
      success: false,
      error: {
        code: RPAErrorCode.ELEMENT_NOT_FOUND,
        message: `Element not found: ${selector}`,
      },
    };
  }

  el.focus();
  el.scrollIntoView({ behavior: "smooth", block: "center" });
  el.value = "";
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.value = text;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));

  return { success: true, data: { typed: text, selector } };
}

function actionRead(params?: Record<string, unknown>): ActionResult {
  const selector = params?.selector as string;
  if (!selector) {
    return {
      success: false,
      error: {
        code: RPAErrorCode.ELEMENT_NOT_FOUND,
        message: "selector is required",
      },
    };
  }

  const el = resolveSelector(selector);
  if (!el) {
    return {
      success: false,
      error: {
        code: RPAErrorCode.ELEMENT_NOT_FOUND,
        message: `Element not found: ${selector}`,
      },
    };
  }

  const htmlEl = el as HTMLElement;
  return {
    success: true,
    data: {
      text: el.textContent?.trim() || "",
      innerHTML: htmlEl.innerHTML,
      value: (el as HTMLInputElement).value || "",
      attributes: getElementAttributes(el),
      tag: el.tagName.toLowerCase(),
      visible: isVisible(el),
    },
  };
}

function actionScroll(params?: Record<string, unknown>): ActionResult {
  const direction = (params?.direction as string) || "down";
  const amount = (params?.amount as number) || 500;
  const scrollY = direction === "up" ? -amount : amount;
  window.scrollBy({ top: scrollY, behavior: "smooth" });
  return {
    success: true,
    data: { direction, amount, scrolledTo: window.scrollY + scrollY },
  };
}

function actionGetState(): ActionResult {
  return {
    success: true,
    data: {
      url: window.location.href,
      title: document.title,
      scrollY: window.scrollY,
      scrollHeight: document.documentElement.scrollHeight,
      viewportHeight: window.innerHeight,
      note: "Use browser_get_state for full state with forms and elements",
    },
  };
}

function actionScreenshot(params?: Record<string, unknown>): ActionResult {
  return {
    success: true,
    data: {
      note: "Screenshot captured by service worker",
      url: window.location.href,
      title: document.title,
      fullPage: params?.fullPage || false,
    },
  };
}

function actionNavigate(params?: Record<string, unknown>): ActionResult {
  const url = params?.url as string;
  if (!url) {
    return {
      success: false,
      error: {
        code: RPAErrorCode.NAVIGATION_ERROR,
        message: "url is required",
      },
    };
  }
  window.location.href = url;
  return { success: true, data: { navigating_to: url } };
}

function actionKeypress(params?: Record<string, unknown>): ActionResult {
  const key = params?.key as string;
  const selector = params?.selector as string | undefined;
  const modifiers = params?.modifiers as string[] | undefined;

  if (!key) {
    return {
      success: false,
      error: { message: "key is required", code: RPAErrorCode.UNKNOWN_ERROR },
    };
  }

  let target: Element = document.activeElement || document.body;
  if (selector) {
    const el = resolveSelector(selector);
    if (el) target = el;
  }

  const eventInit: KeyboardEventInit = {
    key,
    code: key,
    ctrlKey: modifiers?.includes("ctrl") || false,
    shiftKey: modifiers?.includes("shift") || false,
    altKey: modifiers?.includes("alt") || false,
    metaKey: modifiers?.includes("meta") || false,
    bubbles: true,
    cancelable: true,
  };

  target.dispatchEvent(new KeyboardEvent("keydown", eventInit));
  target.dispatchEvent(new KeyboardEvent("keypress", eventInit));
  target.dispatchEvent(new KeyboardEvent("keyup", eventInit));

  return {
    success: true,
    data: { key, modifiers, target: (target as HTMLElement).tagName },
  };
}

function getElementAttributes(el: Element): Record<string, string> {
  const attrs: Record<string, string> = {};
  for (const attr of el.attributes) {
    attrs[attr.name] = attr.value;
  }
  return attrs;
}
