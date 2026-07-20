import { logger } from "../shared/logger";

export function resolveSelector(selectorStr: string): Element | null {
  const trimmed = selectorStr.trim();

  // 1. XPath
  if (trimmed.startsWith("xpath=") || trimmed.startsWith("/")) {
    const xpath = trimmed.startsWith("xpath=")
      ? trimmed.slice(6)
      : trimmed;
    return resolveXPath(xpath);
  }

  // 2. Text match
  if (trimmed.startsWith("text=")) {
    return findByText(trimmed.slice(5).trim());
  }

  // 3. Aria label match
  if (trimmed.startsWith("aria=")) {
    return findByAriaLabel(trimmed.slice(5).trim());
  }

  // 4. CSS selector (default)
  return resolveCSS(trimmed);
}

function resolveCSS(selector: string): Element | null {
  try {
    return document.querySelector(selector);
  } catch {
    logger.warn("Invalid CSS selector:", selector);
    return null;
  }
}

function resolveXPath(xpath: string): Element | null {
  try {
    const result = document.evaluate(
      xpath,
      document,
      null,
      XPathResult.FIRST_ORDERED_NODE_TYPE,
      null
    );
    return result.singleNodeValue as Element | null;
  } catch {
    logger.warn("Invalid XPath:", xpath);
    return null;
  }
}

function findByText(text: string): Element | null {
  const xpath = `//*[normalize-space(text())='${text.replace(/'/g, "\\'")}']`;
  const exact = resolveXPath(xpath);
  if (exact) return exact;

  const allElements = document.querySelectorAll(
    "button, a, input, textarea, select, label, span, p, h1, h2, h3, h4, h5, h6, td, th, li"
  );
  const lowerText = text.toLowerCase();
  for (const el of allElements) {
    const elText = (el.textContent || "").trim().toLowerCase();
    if (elText === lowerText || elText.includes(lowerText)) return el;
  }
  return null;
}

function findByAriaLabel(label: string): Element | null {
  let el = document.querySelector(`[aria-label="${label}"]`);
  if (el) return el;

  const lowerLabel = label.toLowerCase();
  const allWithAria = document.querySelectorAll("[aria-label]");
  for (const ariaEl of allWithAria) {
    const ariaLabel = (ariaEl.getAttribute("aria-label") || "").toLowerCase();
    if (ariaLabel === lowerLabel || ariaLabel.includes(lowerLabel))
      return ariaEl;
  }
  return null;
}

export function generateSelector(el: Element): string {
  if (el.id) return `#${CSS.escape(el.id)}`;

  if (el.classList.length > 0) {
    const sel = `${el.tagName.toLowerCase()}.${Array.from(el.classList).map(CSS.escape).join(".")}`;
    if (document.querySelectorAll(sel).length === 1) return sel;
  }

  for (const attr of el.attributes) {
    if (attr.name.startsWith("data-")) {
      const sel = `[${attr.name}="${attr.value}"]`;
      if (document.querySelectorAll(sel).length === 1) return sel;
    }
  }

  const parts: string[] = [];
  let current: Element | null = el;
  while (current && current !== document.body) {
    let sel = current.tagName.toLowerCase();
    const parent = current.parentElement;
    if (parent) {
      const siblings = Array.from(parent.children).filter(
        (c) => c.tagName === current!.tagName
      );
      if (siblings.length > 1) {
        sel += `:nth-of-type(${siblings.indexOf(current) + 1})`;
      }
    }
    parts.unshift(sel);
    current = current.parentElement;
  }
  return parts.join(" > ");
}

export function isVisible(el: Element): boolean {
  const rect = el.getBoundingClientRect();
  if (rect.width === 0 && rect.height === 0) return false;

  const style = window.getComputedStyle(el);
  if (
    style.display === "none" ||
    style.visibility === "hidden" ||
    style.opacity === "0"
  )
    return false;

  const vh = window.innerHeight || document.documentElement.clientHeight;
  const vw = window.innerWidth || document.documentElement.clientWidth;
  return rect.bottom > 0 && rect.right > 0 && rect.top < vh && rect.left < vw;
}
