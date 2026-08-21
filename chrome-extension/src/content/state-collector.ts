import type { PageState, PageElement, FormInfo } from "../shared/types";
import { isVisible, generateSelector } from "./selector";

export function collectPageState(): PageState {
  return {
    url: window.location.href,
    title: document.title,
    scrollY: window.scrollY,
    scrollHeight: document.documentElement.scrollHeight,
    viewportHeight: window.innerHeight,
    elements: collectInteractiveElements(),
    forms: collectForms(),
  };
}

function collectInteractiveElements(): PageElement[] {
  const selectors =
    'a, button, input, textarea, select, [role="button"], [role="link"], [role="tab"], [role="menuitem"], [contenteditable="true"]';
  const nodes = document.querySelectorAll(selectors);

  return Array.from(nodes).map((el) => ({
    tag: el.tagName.toLowerCase(),
    selector: generateSelector(el),
    text: (el.textContent || "").trim().slice(0, 120),
    role: el.getAttribute("role") || "",
    visible: isVisible(el),
    rect: (() => {
      const r = el.getBoundingClientRect();
      return {
        x: Math.round(r.x),
        y: Math.round(r.y),
        width: Math.round(r.width),
        height: Math.round(r.height),
      };
    })(),
  }));
}

function collectForms(): FormInfo[] {
  const forms = document.querySelectorAll("form");
  return Array.from(forms).map((form) => ({
    selector: generateSelector(form),
    fields: Array.from(form.querySelectorAll("input, textarea, select")).map(
      (field) => {
        const input = field as HTMLInputElement;
        return {
          name: input.name || "",
          type: input.type || field.tagName.toLowerCase(),
          value: input.value || "",
          selector: generateSelector(field),
          label:
            (field.id &&
              document.querySelector(`label[for="${field.id}"]`)
                ?.textContent?.trim()) ||
            field.getAttribute("aria-label") ||
            field.getAttribute("placeholder") ||
            "",
        };
      }
    ),
  }));
}
