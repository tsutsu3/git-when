/**
 * Viewer controls of the HTML report.
 *
 * This module handles language, theme, the sidebar and sheet, the repository list,
 * the author and period filters, and the view tabs.
 * Preferences are remembered only in this browser.
 *
 * @module
 */

import { authorLabels, formatNumber, listedAuthors, projectCounts } from "../report/data";
import { isLang } from "../report/i18n";
import { VIEW_IDS, isViewId, type Lang } from "../report/types";
import { el, type App } from "./dom";
import { drawRunInfo } from "./views";

const LANG_KEY = "git-when:lang";

const THEME_KEY = "git-when:theme";

const SIDE_KEY = "git-when:sidebar";

/** Applies translations to elements with data-i18n and data-i18n-attr. */
export function applyStaticText(app: App): void {
  document.documentElement.lang = app.ui.lang;
  document.querySelectorAll<HTMLElement>("[data-i18n]").forEach((n) => {
    n.textContent = app.t(n.dataset.i18n ?? "");
  });
  document.querySelectorAll<HTMLElement>("[data-i18n-attr]").forEach((n) => {
    // The format is "attr:key;attr:key".
    (n.dataset.i18nAttr ?? "").split(";").forEach((pair) => {
      const [attr, key] = pair.split(":");
      n.setAttribute(attr, app.t(key));
    });
  });
  document.querySelectorAll<HTMLElement>("[data-lang-choice]").forEach((b) => {
    b.setAttribute("aria-pressed", b.dataset.langChoice === app.ui.lang ? "true" : "false");
  });
}

/** Restores the saved language (English by default) and wires the language buttons. */
export function setupLanguage(app: App): void {
  const saved = readPref(LANG_KEY);
  app.ui.lang = isLang(saved) ? saved : "en";
  document.querySelectorAll<HTMLElement>("[data-lang-choice]").forEach((b) => {
    b.addEventListener("click", () => {
      const next = b.dataset.langChoice;
      if (isLang(next)) setLang(app, next);
    });
  });
  applyStaticText(app);
}

/** Restores the saved theme (system by default) and wires the theme buttons. */
export function setupTheme(app: App): void {
  const saved = readPref(THEME_KEY);
  document.querySelectorAll<HTMLElement>("[data-theme-choice]").forEach((b) => {
    b.addEventListener("click", () => setTheme(app, b.dataset.themeChoice));
  });
  setTheme(app, saved || "system");
}

/** Applies the sidebar or sheet state to the page and updates the toggle labels. */
export function applySidebar(app: App): void {
  const { els, ui } = app;
  const narrow = isNarrow(app);
  els.shell.classList.toggle("side-closed", !narrow && !ui.sideOpen);
  els.shell.classList.toggle("sheet-open", narrow && ui.sheetOpen);
  els.sheetBackdrop.hidden = !(narrow && ui.sheetOpen);

  const open = narrow ? ui.sheetOpen : ui.sideOpen;
  const label = app.t(open ? "hideProjects" : "openProjects");
  els.sideToggle.setAttribute("aria-expanded", open ? "true" : "false");
  els.sideToggle.setAttribute("aria-label", label);
  els.sideToggle.title = label;
  // The sheet covers the top bar, so the sheet has its own close button at the same position.
  els.sideCollapse.setAttribute("aria-label", app.t("hideProjects"));
  els.sideCollapse.title = app.t("hideProjects");
}

/** Restores the sidebar state and wires the toggle, the sheet, and the breakpoint. */
export function setupSidebar(app: App): void {
  const { els } = app;
  app.ui.sideOpen = readPref(SIDE_KEY) !== "closed";

  els.sideToggle.addEventListener("click", () => {
    if (isNarrow(app)) setSheet(app, !app.ui.sheetOpen);
    else setSidebar(app, !app.ui.sideOpen);
  });
  els.sideCollapse.addEventListener("click", () => setSheet(app, false));
  els.sheetBackdrop.addEventListener("click", () => setSheet(app, false));
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && app.ui.sheetOpen) setSheet(app, false);
  });
  // Do not leave an open sheet behind when the window crosses the breakpoint.
  const query = app.narrowQuery;
  if (query) {
    const onChange = () => {
      app.ui.sheetOpen = false;
      applySidebar(app);
      syncTopbarHeight(app);
    };
    if (query.addEventListener) query.addEventListener("change", onChange);
    else query.addListener(onChange);
  }
  window.addEventListener("resize", () => syncTopbarHeight(app));
  applySidebar(app);
  syncTopbarHeight(app);
  // Enable animations only after the saved state is applied.
  requestAnimationFrame(() => els.shell.classList.add("side-ready"));
}

/** Draws the repository list with commit counts for the current author and year. */
export function drawRoster(app: App): void {
  const { ctx, els, sel } = app;
  const projects = ctx.data.projects;
  els.roster.innerHTML = "";
  els.rosterCount.textContent = formatNumber(projects.length);

  const counts = projectCounts(ctx.data, sel);
  const max = counts.reduce((m, n) => Math.max(m, n), 1);
  const item = (idx: number, name: string, n: number) => {
    const li = document.createElement("li");
    const b = el("button", "proj" + (idx < 0 ? " all" : "") + (n === 0 ? " zero" : ""));
    b.type = "button";
    b.title = name;
    b.setAttribute("aria-current", sel.project === idx ? "true" : "false");
    const text = el("span", "proj-text");
    // Show the directory part of "dir/name" on its own smaller line.
    const slash = idx >= 0 ? name.lastIndexOf("/") : -1;
    if (slash > 0) text.appendChild(el("span", "proj-dir", name.slice(0, slash + 1)));
    text.appendChild(el("span", "proj-base", slash > 0 ? name.slice(slash + 1) : name));
    b.appendChild(text);
    b.appendChild(el("span", "proj-n", formatNumber(n)));
    if (idx >= 0) {
      const bar = el("span", "proj-bar");
      const fill = el("i");
      fill.style.width = (n / max) * 100 + "%";
      bar.appendChild(fill);
      b.appendChild(bar);
    }
    b.addEventListener("click", () => {
      sel.project = idx;
      app.render();
      // On narrow screens, close the sheet to show the result.
      if (app.ui.sheetOpen) setSheet(app, false);
    });
    li.appendChild(b);
    return li;
  };

  els.roster.appendChild(
    item(
      -1,
      app.t("allProjects"),
      counts.reduce((s, n) => s + n, 0),
    ),
  );
  const q = sel.rosterQuery.trim().toLowerCase();
  let shown = 0;
  for (const i of ctx.rosterOrder) {
    const name = projects[i].name;
    if (q && !name.toLowerCase().includes(q)) continue;
    els.roster.appendChild(item(i, name, counts[i]));
    shown++;
  }
  if (q && !shown) els.roster.appendChild(el("li", "roster-empty", app.t("noProjectMatch")));
}

/**
 * Rebuilds the author and period options in the current language.
 *
 * Authors are sorted by commits. Authors that share a name are told apart as in authorLabels.
 * Authors under --min-total are left out, and a disabled option says how many.
 */
export function fillFilterOptions(app: App): void {
  const { ctx, els } = app;
  const authors = ctx.data.authors;
  const labels = authorLabels(ctx);

  els.author.innerHTML = "";
  const all = el("option", null, app.t("allAuthors", { n: formatNumber(authors.length) }));
  all.value = "-1";
  els.author.appendChild(all);
  const listed = listedAuthors(ctx);
  listed.forEach((i) => {
    const a = authors[i];
    const label = labels[i];
    // The "All authors" option shows a number of people in parentheses.
    // Commit counts use a unit instead, so the two numbers are not mixed up.
    const text = app.t("authorOption", { name: label, n: formatNumber(ctx.authorTotals[i]) });
    const o = el("option", null, text);
    o.value = String(i);
    // With git-when --no-email the email is empty, so the title is the label.
    o.title = a.email ? `${a.name} <${a.email}>` : label;
    els.author.appendChild(o);
  });
  const hidden = authors.length - listed.length;
  if (hidden > 0) {
    const note = el(
      "option",
      null,
      app.t("hiddenAuthors", {
        n: formatNumber(hidden),
        min: formatNumber(ctx.data.meta.minTotal || 0),
      }),
    );
    note.disabled = true;
    els.author.appendChild(note);
  }

  els.year.innerHTML = "";
  const any = el("option", null, app.t("allTime"));
  any.value = "-1";
  els.year.appendChild(any);
  for (let yi = ctx.data.years.length - 1; yi >= 0; yi--) {
    const o = el("option", null, app.t("year", { y: ctx.data.years[yi] }));
    o.value = String(yi);
    els.year.appendChild(o);
  }
}

/** Fills and wires the author, period, clear, and repository search controls. */
export function setupFilters(app: App): void {
  const { els, sel } = app;
  fillFilterOptions(app);
  els.author.addEventListener("change", () => {
    sel.author = Number(els.author.value);
    app.render();
  });
  els.year.addEventListener("change", () => {
    sel.year = Number(els.year.value);
    app.render();
  });
  els.reset.addEventListener("click", () => {
    sel.author = -1;
    sel.project = -1;
    sel.year = -1;
    app.render();
  });
  els.rosterSearch.addEventListener("input", () => {
    sel.rosterQuery = els.rosterSearch.value;
    drawRoster(app);
  });
}

/** Wires the view tabs. Arrow keys, Home, and End also switch tabs (the WAI-ARIA tabs pattern). */
export function setupTabs(app: App): void {
  const last = VIEW_IDS.length - 1;
  for (const view of VIEW_IDS) {
    const tab = app.els.tabs[view];
    tab.addEventListener("click", () => {
      const next = tab.dataset.view;
      if (isViewId(next)) app.sel.view = next;
      app.render();
    });
    tab.addEventListener("keydown", (e) => {
      const i = VIEW_IDS.indexOf(view);
      const target: Record<string, number> = {
        ArrowRight: i === last ? 0 : i + 1,
        ArrowLeft: i === 0 ? last : i - 1,
        Home: 0,
        End: last,
      };
      const next = target[e.key];
      if (next === undefined) return;
      e.preventDefault();
      app.sel.view = VIEW_IDS[next];
      app.render();
      app.els.tabs[app.sel.view].focus();
    });
  }
}

// Storage can be unavailable, for example in some file:// or private browsing setups.
// Preferences then last only for this page view.
function readPref(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writePref(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Nothing to do. The preference is not remembered.
  }
}

function setLang(app: App, next: Lang): void {
  app.ui.lang = next;
  writePref(LANG_KEY, next);
  applyStaticText(app);
  fillFilterOptions(app);
  drawRunInfo(app);
  applySidebar(app);
  app.render();
}

function setTheme(app: App, pref: string | null | undefined): void {
  app.ui.theme = pref === "light" || pref === "dark" ? pref : "system";
  const root = document.documentElement;
  if (app.ui.theme === "system") delete root.dataset.theme;
  else root.dataset.theme = app.ui.theme;
  document.querySelectorAll<HTMLElement>("[data-theme-choice]").forEach((b) => {
    b.setAttribute("aria-pressed", b.dataset.themeChoice === app.ui.theme ? "true" : "false");
  });
  writePref(THEME_KEY, app.ui.theme);
}

// On wide screens the sidebar opens and closes, and the state is remembered.
// The toggle button stays at the same top-left position and only its icon and label change.
// On narrow screens the list becomes a sheet from the left, and its state is not remembered.

const isNarrow = (app: App): boolean => !!(app.narrowQuery && app.narrowQuery.matches);

/** Opens or closes the sheet, and moves focus to the filter box or back to the toggle. */
function setSheet(app: App, open: boolean): void {
  app.ui.sheetOpen = open;
  applySidebar(app);
  (open ? app.els.rosterSearch : app.els.sideToggle).focus();
}

function setSidebar(app: App, open: boolean): void {
  app.ui.sideOpen = open;
  applySidebar(app);
  writePref(SIDE_KEY, open ? "open" : "closed");
}

/** Stores the real top bar height in --topbar-h. The sidebar uses it for its position and height. */
function syncTopbarHeight(app: App): void {
  document.documentElement.style.setProperty("--topbar-h", app.els.topbar.offsetHeight + "px");
}
