/**
 * Page elements, the shared app object, and small DOM helpers.
 *
 * @module
 */

import { formatNumber, levelOf, levelSteps, type ReportContext } from "../report/data";
import type { MessageParams } from "../report/i18n";
import {
  VIEW_IDS,
  type Lang,
  type Selection,
  type ThemePreference,
  type ViewId,
} from "../report/types";

/** The elements returned by {@link queryReportElements}. */
export type ReportElements = ReturnType<typeof queryReportElements>;

/** Viewer preferences that are not part of the data selection. */
export interface UiState {
  lang: Lang;
  theme: ThemePreference;
  /** Whether the sidebar is shown on wide screens. */
  sideOpen: boolean;
  /** Whether the repository sheet is open on narrow screens. */
  sheetOpen: boolean;
}

/**
 * The running report.
 *
 * UI modules receive it instead of reading globals, so they never import the entry point.
 */
export interface App {
  ctx: ReportContext;
  els: ReportElements;
  sel: Selection;
  ui: UiState;
  /** Matches narrow screens. It is the same breakpoint as the CSS (880px). */
  narrowQuery: MediaQueryList | null;
  /** Translates a message in the current language. */
  t(key: string, params?: MessageParams): string;
  /** Returns the short weekday name in the current language. */
  day(d: number): string;
  /** Shows `html` in the tooltip while the pointer is over `node`. */
  bindTip(node: HTMLElement, html: string): void;
  /** Draws everything that depends on the selection. */
  render(): void;
}

type ElementType<T extends Element> = abstract new (...args: never[]) => T;

/**
 * Returns the element with `id`.
 *
 * @throws When the element is missing or has another type. The message names the element.
 */
export function requireElement<T extends HTMLElement>(id: string, type: ElementType<T>): T {
  return checked(document.getElementById(id), `#${id}`, type);
}

/** Returns the first element that matches `selector`. It throws like {@link requireElement}. */
export function requireSelector<T extends HTMLElement>(selector: string, type: ElementType<T>): T {
  return checked(document.querySelector(selector), selector, type);
}

/**
 * Finds every element that the report script uses.
 *
 * Call it once at startup, so a markup change that breaks the script fails early with a clear message.
 */
export function queryReportElements() {
  return {
    shell: requireSelector(".shell", HTMLElement),
    topbar: requireSelector(".topbar", HTMLElement),
    sideToggle: requireElement("side-toggle", HTMLButtonElement),
    sideCollapse: requireElement("side-collapse", HTMLButtonElement),
    sheetBackdrop: requireElement("sheet-backdrop", HTMLElement),
    author: requireElement("author", HTMLSelectElement),
    year: requireElement("year", HTMLSelectElement),
    reset: requireElement("reset", HTMLButtonElement),
    githubLink: requireElement("github-link", HTMLAnchorElement),
    roster: requireElement("roster", HTMLUListElement),
    rosterCount: requireElement("roster-count", HTMLElement),
    rosterSearch: requireElement("roster-search", HTMLInputElement),
    context: requireElement("context", HTMLElement),
    lede: requireElement("lede", HTMLElement),
    kpis: requireElement("kpis", HTMLElement),
    tabs: byView("tab-", HTMLButtonElement),
    panels: byView("p-", HTMLElement),
    hm: requireElement("hm", HTMLElement),
    hmLegend: requireElement("hm-legend", HTMLElement),
    wh: requireElement("wh", HTMLElement),
    wkLeftHead: requireElement("wk-left-head", HTMLElement),
    wkRightHead: requireElement("wk-right-head", HTMLElement),
    wkRows: requireElement("wk-rows", HTMLElement),
    wkTotal: requireElement("wk-total", HTMLElement),
    wkIndex: requireElement("wk-index", HTMLElement),
    mo: requireElement("mo", HTMLElement),
    moLegend: requireElement("mo-legend", HTMLElement),
    tzs: requireElement("tzs", HTMLElement),
    tzLegend: requireElement("tz-legend", HTMLElement),
    cmd: requireElement("cmd", HTMLElement),
    heads: requireElement("heads", HTMLTableElement),
    tip: requireElement("tip", HTMLElement),
  };
}

/** Creates an element with an optional class name and text. */
export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  cls?: string | null,
  txt?: string | number | null,
): HTMLElementTagNameMap[K] {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (txt != null) n.textContent = String(txt);
  return n;
}

/** Returns a function that binds tooltip content to elements. The tooltip follows the pointer. */
export function createTooltip(tip: HTMLElement): (node: HTMLElement, html: string) => void {
  return (node, html) => {
    node.addEventListener("mouseenter", () => {
      tip.innerHTML = html;
      tip.classList.add("on");
    });
    node.addEventListener("mousemove", (e) => {
      const pad = 14;
      let x = e.clientX + pad;
      let y = e.clientY + pad;
      const r = tip.getBoundingClientRect();
      // Flip to the other side of the pointer near the window edges.
      if (x + r.width > innerWidth - 8) x = e.clientX - r.width - pad;
      if (y + r.height > innerHeight - 8) y = e.clientY - r.height - pad;
      tip.style.left = x + "px";
      tip.style.top = y + "px";
    });
    node.addEventListener("mouseleave", () => tip.classList.remove("on"));
  };
}

/** Creates a heat cell shaded for `v` relative to `max`. */
export const heatCell = (v: number, max: number): HTMLDivElement =>
  el("div", "cell l" + levelOf(v, max));

/** Draws the shading legend, with the smallest count of each level, followed by `note`. */
export function drawLegend(box: HTMLElement, max: number, note: string): void {
  box.innerHTML = "";
  const scale = el("div", "legend-scale");
  const item = (level: number, label: string) => {
    const s = el("span", "sw-item");
    s.appendChild(el("i", "sw l" + level));
    s.appendChild(el("span", null, label));
    scale.appendChild(s);
  };
  item(0, "0");
  levelSteps(max).forEach((s) => item(s.level, `${formatNumber(s.min)}+`));
  box.appendChild(scale);
  box.appendChild(el("span", "legend-note", note));
}

function checked<T extends HTMLElement>(
  node: Element | null,
  name: string,
  type: ElementType<T>,
): T {
  if (node instanceof type) return node;
  const found = node ? `<${node.tagName.toLowerCase()}>` : "nothing";
  throw new Error(
    `git-when: required element ${name} is missing (expected ${type.name}, found ${found})`,
  );
}

function byView<T extends HTMLElement>(prefix: string, type: ElementType<T>): Record<ViewId, T> {
  return Object.fromEntries(VIEW_IDS.map((v) => [v, requireElement(prefix + v, type)])) as Record<
    ViewId,
    T
  >;
}
