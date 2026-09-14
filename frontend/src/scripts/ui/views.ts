/**
 * Drawing of the summary, the five visualizations, and the run info.
 *
 * Each draw function rebuilds its part of the page from the current selection.
 *
 * @module
 */

import {
  authorLabels,
  escapeHtml,
  firstActiveYear,
  formatNumber,
  formatOffset,
  offsetTable,
  pad2,
  percent,
  verdictLevel,
  type Marginals,
  type WeekStats,
} from "../report/data";
import { MONTH_NAMES_EN } from "../report/i18n";
import { drawLegend, el, heatCell, type App } from "./dom";

/** Draws the view that is selected. */
export function drawView(app: App, m: Marginals, st: WeekStats): void {
  switch (app.sel.view) {
    case "heatmap":
      return drawHeatmap(app, m, st);
    case "weekday-hours":
      return drawWeekdayHours(app, m, st);
    case "week":
      return drawWeek(app, m, st);
    case "month":
      return drawMonth(app, m);
    case "tzshift":
      return drawTZShift(app);
  }
}

/** Draws the selection line, the summary sentence, and the KPI tiles. */
export function drawSummary(app: App, m: Marginals, st: WeekStats): void {
  const { ctx, els, sel } = app;
  const { data } = ctx;
  const project =
    sel.project < 0 || !data.projects[sel.project]
      ? app.t("allProjects")
      : data.projects[sel.project].name;
  const author =
    sel.author < 0 || !data.authors[sel.author]
      ? app.t("anyAuthor")
      : authorLabels(ctx)[sel.author];
  const period =
    sel.year < 0
      ? data.meta.period || app.t("allTime")
      : app.t("year", { y: data.years[sel.year] });

  els.context.innerHTML = "";
  [project, author, period].forEach((s, i) => {
    if (i) els.context.appendChild(document.createTextNode(" · "));
    els.context.appendChild(el("b", null, s));
  });
  els.context.appendChild(document.createTextNode(" · " + app.t("localTime")));

  if (!m.total) {
    els.lede.textContent = app.t("noCommits");
  } else {
    els.lede.innerHTML = app.t("lede", {
      when: escapeHtml(app.t("ledeWhen", { day: app.day(st.peak.d), h: pad2(st.peak.h) })),
      verdict: escapeHtml(app.t("verdict." + verdictLevel(st.index))),
    });
  }

  const box = els.kpis;
  box.innerHTML = "";
  const tile = (label: string, value: string, sub: string) => {
    const k = el("div", "kpi");
    k.appendChild(el("div", "kpi-label", label));
    k.appendChild(el("div", "kpi-value", value));
    k.appendChild(el("div", "kpi-sub", sub));
    box.appendChild(k);
  };
  if (!m.total) {
    tile(app.t("kpi.commits"), "0", app.t("kpi.none"));
    return;
  }
  tile(
    app.t("kpi.commits"),
    formatNumber(m.total),
    app.t("kpi.commitsSub", {
      p: formatNumber(m.projects),
      a: formatNumber(m.authors),
    }),
  );
  tile(
    app.t("kpi.peak"),
    app.t("dayHour", { day: app.day(st.peak.d), h: pad2(st.peak.h) }),
    commitsText(app, st.peak.c),
  );
  tile(
    app.t("kpi.index"),
    st.index.toFixed(2),
    app.t("kpi.indexSub", { share: percent(st.share), base: percent(st.base) }),
  );
  tile(app.t("kpi.night"), percent(st.nightShare), commitsText(app, st.nightN));
}

/** Draws the repository heads and the command line that generated the report. */
export function drawRunInfo(app: App): void {
  const { ctx, els } = app;
  const { data } = ctx;
  els.heads.innerHTML = "";
  for (const i of ctx.rosterOrder) {
    const p = data.projects[i];
    const tr = document.createElement("tr");
    tr.appendChild(el("td", null, p.name));
    tr.appendChild(el("td", "mono", p.head ? p.head.slice(0, 12) : "-"));
    tr.appendChild(el("td", null, commitsText(app, p.commits)));
    els.heads.appendChild(tr);
  }
  const meta = data.meta;
  els.cmd.textContent = [
    meta.command,
    `${meta.tool || "git-when"} ${meta.version || ""}`.trim(),
    app.t("runInfoLine", {
      tz: app.t("tz." + meta.tz),
      period: meta.period || "-",
      n: data.projects.length,
    }),
  ]
    .filter(Boolean)
    .join("\n");
}

const commitsText = (app: App, n: number) => app.t("commits", { n: formatNumber(n) });

const shareText = (v: number, total: number) =>
  ` <span class="k">/ ${total ? percent(v / total) : "0%"}</span>`;

const yearMonthText = (app: App, y: number, m: number) =>
  app.t("yearMonth", { y, m: m + 1, mon: MONTH_NAMES_EN[m] });

function drawHeatmap(app: App, m: Marginals, st: WeekStats): void {
  const { wh, total, byProject } = m;
  const g = app.els.hm;
  g.innerHTML = "";

  let max = 0;
  for (let d = 0; d < 7; d++) for (let h = 0; h < 24; h++) max = Math.max(max, wh[d][h]);
  const hourMax = Math.max(1, ...st.hourTotals);

  // Hourly totals as bars above the grid.
  g.appendChild(el("div"));
  for (let h = 0; h < 24; h++) {
    const col = el("div", "colbar");
    const bar = el("i");
    bar.style.height = (st.hourTotals[h] / hourMax) * 100 + "%";
    col.appendChild(bar);
    app.bindTip(
      col,
      `<b>${app.t("hourSlot", { h: pad2(h) })}</b><br>${commitsText(app, st.hourTotals[h])}` +
        shareText(st.hourTotals[h], total),
    );
    g.appendChild(col);
  }
  g.appendChild(el("div"));

  g.appendChild(el("div"));
  for (let h = 0; h < 24; h++) g.appendChild(el("div", "tick" + (h % 3 ? " minor" : ""), pad2(h)));
  g.appendChild(el("div", "tick", app.t("total")));

  for (const d of app.ctx.dayOrder) {
    const weekend = app.ctx.weekend.includes(d);
    g.appendChild(el("div", "rowlabel" + (weekend ? " strong" : ""), app.day(d)));
    for (let h = 0; h < 24; h++) {
      const v = wh[d][h];
      const c = heatCell(v, max);
      app.bindTip(
        c,
        `<b>${app.t("dayHour", { day: app.day(d), h: pad2(h) })}</b><br>${commitsText(app, v)}` +
          shareText(v, total) +
          (v && byProject ? topProjects(app, byProject[d * 24 + h]) : ""),
      );
      g.appendChild(c);
    }
    g.appendChild(el("div", "rowtotal", formatNumber(st.dayTotals[d])));
  }

  drawLegend(
    app.els.hmLegend,
    max,
    max
      ? app.t("peakAt", {
          where: app.t("dayHour", {
            day: app.day(st.peak.d),
            h: pad2(st.peak.h),
          }),
          n: formatNumber(st.peak.c),
        })
      : app.t("noCommitsShort"),
  );
}

/** Returns tooltip HTML for the three projects with the most commits in a cell. */
function topProjects(app: App, counts: Map<number, number>): string {
  const projects = app.ctx.data.projects;
  if (projects.length < 2) return "";
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1] || a[0] - b[0])
    .slice(0, 3)
    .map(
      ([p, c]) =>
        `<br><span class="k">${escapeHtml(projects[p] ? projects[p].name : "?")}</span> ${formatNumber(c)}`,
    )
    .join("");
}

function drawWeekdayHours(app: App, m: Marginals, st: WeekStats): void {
  const { wh } = m;
  const g = app.els.wh;
  g.innerHTML = "";

  let max = 0;
  const peaks = new Array<number>(7).fill(0);
  for (let d = 0; d < 7; d++) {
    for (let h = 0; h < 24; h++) {
      max = Math.max(max, wh[d][h]);
      if (wh[d][h] > wh[d][peaks[d]]) peaks[d] = h;
    }
  }

  g.appendChild(el("div", "head"));
  for (const d of app.ctx.dayOrder) {
    const head = el("div", "head");
    const weekend = app.ctx.weekend.includes(d);
    head.appendChild(
      el("b", null, app.t(weekend ? "dayHeadWeekend" : "dayHead", { day: app.day(d) })),
    );
    head.appendChild(
      el(
        "span",
        null,
        st.dayTotals[d]
          ? app.t("dayPeak", {
              n: formatNumber(st.dayTotals[d]),
              h: pad2(peaks[d]),
            })
          : "0",
      ),
    );
    g.appendChild(head);
  }

  // All weekdays share one scale. Comparing days is the point of this view.
  for (let h = 0; h < 24; h++) {
    g.appendChild(el("div", "hr", pad2(h)));
    for (const d of app.ctx.dayOrder) {
      const v = wh[d][h];
      const cell = el("div", "bar-cell");
      const bar = el("div", "bar");
      bar.style.width = max ? (v / max) * 78 + "%" : "0";
      cell.appendChild(bar);
      cell.appendChild(el("span", "v", formatNumber(v)));
      g.appendChild(cell);
    }
  }
}

function drawWeek(app: App, m: Marginals, st: WeekStats): void {
  const { els, ctx } = app;
  const { wh, total } = m;
  els.wkRows.innerHTML = "";

  const weekendDays = ctx.weekend.length;
  els.wkLeftHead.textContent = app.t("weekdaysHead", { n: 7 - weekendDays });
  els.wkRightHead.textContent = app.t("weekendHead", {
    days: ctx.dayOrder
      .filter((d) => ctx.weekend.includes(d))
      .map((d) => app.day(d))
      .join(app.t("daySep")),
    n: weekendDays,
  });

  const wd = new Array<number>(24).fill(0);
  const we = new Array<number>(24).fill(0);
  for (let d = 0; d < 7; d++) {
    const side = ctx.weekend.includes(d) ? we : wd;
    for (let h = 0; h < 24; h++) side[h] += wh[d][h];
  }
  // Both sides share one scale. Separate scales would make a quiet weekend look as busy as weekdays.
  const max = Math.max(1, ...wd, ...we);

  const side = (cls: string, v: number) => {
    const s = el("div", "wk-side " + cls);
    const bar = el("div", "bar");
    bar.style.width = (v / max) * 88 + "%";
    s.appendChild(bar);
    s.appendChild(el("span", "v", formatNumber(v)));
    return s;
  };
  for (let h = 0; h < 24; h++) {
    const row = el("div", "wk-row");
    row.appendChild(side("left", wd[h]));
    row.appendChild(el("div", "hr", pad2(h)));
    row.appendChild(side("right", we[h]));
    els.wkRows.appendChild(row);
  }

  const wdN = total - st.weekendN;
  els.wkTotal.innerHTML = "";
  const tot = (cls: string, n: number, share: number) => {
    const d = el("div", cls);
    d.appendChild(el("b", null, formatNumber(n)));
    d.appendChild(el("span", null, ` (${total ? percent(share) : "0%"})`));
    return d;
  };
  els.wkTotal.appendChild(tot("l", wdN, total ? wdN / total : 0));
  els.wkTotal.appendChild(el("div"));
  els.wkTotal.appendChild(tot("", st.weekendN, st.share));

  // The gauge runs from 0 to 2.00, with 1.00 (even) in the middle.
  const idx = st.index;
  els.wkIndex.innerHTML = `<div class="big">${idx.toFixed(2)}<small>${escapeHtml(app.t("indexLabel"))}</small></div>
     <p>${escapeHtml(app.t("indexText"))}</p>
     <div class="gauge" aria-hidden="true">
 <div class="gauge-track"><div class="fill" style="width:${Math.min(idx / 2, 1) * 100}%"></div><div class="ref"></div></div>
 <div class="gauge-scale"><span>0</span><span>${escapeHtml(app.t("gaugeEven"))}</span><span>2.00+</span></div>
     </div>`;
}

/** Creates a year label that toggles the year filter. */
function yearButton(app: App, yearIndex: number): HTMLButtonElement {
  const selected = app.sel.year === yearIndex;
  const label = el("button", "row-label", String(app.ctx.data.years[yearIndex]));
  label.type = "button";
  label.setAttribute("aria-pressed", selected ? "true" : "false");
  label.addEventListener("click", () => {
    app.sel.year = selected ? -1 : yearIndex;
    app.render();
  });
  return label;
}

function drawMonth(app: App, m: Marginals): void {
  const { ym } = m;
  const years = app.ctx.data.years;
  const g = app.els.mo;
  g.innerHTML = "";

  g.appendChild(el("div", "corner"));
  for (let mo = 0; mo < 12; mo++)
    g.appendChild(el("div", "tick", app.ui.lang === "en" ? MONTH_NAMES_EN[mo] : String(mo + 1)));
  g.appendChild(el("div", "tick", app.t("total")));

  // A year by month heatmap. Shading and the legend work like the heatmap view.
  let max = 0;
  let peak: { yi: number; m: number } | null = null;
  ym.forEach((r, yi) =>
    r.forEach((v, mo) => {
      if (v > max) {
        max = v;
        peak = { yi, m: mo };
      }
    }),
  );

  // Empty years before the first activity are skipped. The selected year stays, so it can be cleared.
  const sel = app.sel.year;
  const first = sel >= 0 ? Math.min(sel, firstActiveYear(ym)) : firstActiveYear(ym);
  years.forEach((yr, yi) => {
    if (yi < first) return;
    g.appendChild(yearButton(app, yi));
    let tot = 0;
    for (let mo = 0; mo < 12; mo++) {
      const v = ym[yi][mo];
      tot += v;
      const c = heatCell(v, max);
      app.bindTip(c, `<b>${escapeHtml(yearMonthText(app, yr, mo))}</b><br>${commitsText(app, v)}`);
      g.appendChild(c);
    }
    const selected = app.sel.year === yi;
    g.appendChild(el("div", "rowtotal" + (selected ? " sel" : ""), formatNumber(tot)));
  });

  const top = peak as { yi: number; m: number } | null;
  drawLegend(
    app.els.moLegend,
    max,
    top
      ? app.t("peakAt", {
          where: yearMonthText(app, years[top.yi], top.m),
          n: formatNumber(max),
        })
      : app.t("noCommitsShort"),
  );
}

function drawTZShift(app: App): void {
  const { els } = app;
  const years = app.ctx.data.years;
  els.tzs.innerHTML = "";
  const table = offsetTable(app.ctx.data, app.sel);
  if (!table.offsets.length) {
    els.tzs.textContent = app.t("noData");
    els.tzLegend.innerHTML = "";
    return;
  }

  // Many offsets mean many columns. Columns have a fixed width and the frame scrolls sideways.
  const tbl = el("div", "tz");
  tbl.style.gridTemplateColumns = `58px repeat(${table.offsets.length}, 44px) auto`;
  tbl.appendChild(el("div", "corner"));
  table.offsets.forEach((o) => tbl.appendChild(el("div", "tick", formatOffset(o))));
  tbl.appendChild(el("div", "tick", app.t("most")));
  for (const y of table.years) {
    tbl.appendChild(yearButton(app, y));
    let yearTotal = 0;
    let top: { o: number; v: number } | null = null;
    for (const o of table.offsets) {
      const v = table.count(y, o);
      yearTotal += v;
      if (v && (!top || v > top.v)) top = { o, v };
      const c = heatCell(v, table.max);
      app.bindTip(
        c,
        `<b>${escapeHtml(app.t("yearOffset", { y: years[y], o: formatOffset(o) }))}</b><br>${commitsText(app, v)}`,
      );
      tbl.appendChild(c);
    }
    tbl.appendChild(
      el("div", "top", top ? `${formatOffset(top.o)} · ${percent(top.v / yearTotal)}` : "-"),
    );
  }
  els.tzs.appendChild(tbl);
  drawLegend(els.tzLegend, table.max, app.t("maxCommits", { n: formatNumber(table.max) }));
}
