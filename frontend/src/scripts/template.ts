/**
 * Entry point of the HTML report.
 *
 * It loads the data, finds the page elements, wires up the controls, and draws the report.
 * Astro bundles this module and its imports into the one application script of the page.
 *
 * The data comes from `window.__GIT_WHEN_DATA__`.
 * Go replaces the data script of the generated template with the real data.
 * During development and in the plain template, `sample.ts` sets sample data instead.
 *
 * @module
 */

import { loadReportData, marginalize, weekStats } from "./report/data";
import { dayName, translate } from "./report/i18n";
import { ALL, VIEW_IDS } from "./report/types";
import {
  drawRoster,
  setupFilters,
  setupLanguage,
  setupSidebar,
  setupTabs,
  setupTheme,
} from "./ui/controls";
import { createTooltip, queryReportElements, type App } from "./ui/dom";
import { drawRunInfo, drawSummary, drawView } from "./ui/views";

const ctx = loadReportData(window.__GIT_WHEN_DATA__);
const els = queryReportElements();

const app: App = {
  ctx,
  els,
  // --default-author decides the author selected when the page opens.
  sel: { project: ALL, author: ctx.defaultAuthor, year: ALL, view: "heatmap", rosterQuery: "" },
  ui: { lang: "en", theme: "system", sideOpen: true, sheetOpen: false },
  narrowQuery: window.matchMedia ? window.matchMedia("(max-width: 880px)") : null,
  t: (key, params) => translate(app.ui.lang, key, params),
  day: (d) => dayName(app.ui.lang, d),
  bindTip: createTooltip(els.tip),
  render,
};

function render(): void {
  const m = marginalize(ctx.data, app.sel);
  const st = weekStats(m.wh, m.total, ctx.weekend);

  els.author.value = String(app.sel.author);
  els.year.value = String(app.sel.year);
  els.reset.hidden = app.sel.author < 0 && app.sel.project < 0 && app.sel.year < 0;

  drawRoster(app);
  drawSummary(app, m, st);
  drawView(app, m, st);

  for (const view of VIEW_IDS) {
    const on = view === app.sel.view;
    els.tabs[view].setAttribute("aria-selected", on ? "true" : "false");
    els.tabs[view].tabIndex = on ? 0 : -1;
    els.panels[view].hidden = !on;
  }
}

// git-when --no-github-link leaves out the link to the repository.
els.githubLink.hidden = Boolean(ctx.data.meta.noGitHubLink);
setupLanguage(app);
setupTheme(app);
setupSidebar(app);
setupFilters(app);
setupTabs(app);
drawRunInfo(app);
render();
