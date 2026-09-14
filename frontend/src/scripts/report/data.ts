/**
 * Data loading, aggregation, and formatting for the HTML report.
 *
 * Nothing in this module touches the DOM, so unit tests can import it directly.
 *
 * @module
 */

import { ALL, type ReportData, type Selection } from "./types";

/** The data plus values derived from it once at startup. */
export interface ReportContext {
  data: ReportData;
  /** Weekend days, 0=Mon..6=Sun. */
  weekend: number[];
  /** First day of the week, 0=Mon..6=Sun. */
  weekStart: number;
  /** Weekday numbers in display order. */
  dayOrder: number[];
  /**
   * Project indexes by total commits, largest first.
   * The order stays fixed so the list does not jump when filters change.
   */
  rosterOrder: number[];
  /** Total commits per author index. */
  authorTotals: number[];
  /** The author index selected when the page opens (--default-author), or ALL. */
  defaultAuthor: number;
}

/** Counts for the current selection. */
export interface Marginals {
  /** Commits as `wh[weekday][hour]`, with every filter applied. */
  wh: number[][];
  /**
   * Commits as `ym[yearIndex][month]`.
   * The year filter is not applied, so every year stays visible and the selected year is highlighted.
   */
  ym: number[][];
  total: number;
  /**
   * Commits per project for each cell (`weekday * 24 + hour`).
   * It exists only when all projects are shown, for the top projects in tooltips.
   */
  byProject: Map<number, number>[] | null;
  /** Number of distinct authors in the selection. */
  authors: number;
  /** Number of distinct projects in the selection. */
  projects: number;
}

/** Summary values of a weekday by hour table. */
export interface WeekStats {
  /** The busiest cell. `c` is 0 when there are no commits. */
  peak: { d: number; h: number; c: number };
  weekendN: number;
  nightN: number;
  /** Share of commits on weekend days. */
  share: number;
  /** Weekend share if every day were equally busy (weekend days / 7). */
  base: number;
  hourTotals: number[];
  dayTotals: number[];
  /** Weekend index. 1.00 means weekend days are as busy per day as weekdays. */
  index: number;
  /** Share of commits in the night hours. */
  nightShare: number;
}

/** A year by UTC offset table for the time zone view. */
export interface OffsetTable {
  /** Offsets in seconds, in ascending order. */
  offsets: number[];
  /** Year indexes with commits, in ascending order. */
  years: number[];
  /** The largest cell. */
  max: number;
  /** Returns the commits for a year index and an offset. */
  count(year: number, offset: number): number;
}

/** One legend entry. `min` is the smallest count shown with `level`. */
export interface LevelStep {
  level: number;
  min: number;
}

/** Number of array elements per bucket record. */
export const BUCKET_STRIDE = 7;

/** Number of array elements per offset record. */
export const OFFSET_STRIDE = 5;

/** Saturday and Sunday, used when the data has no weekend. */
export const DEFAULT_WEEKEND = [5, 6];

/** Number of shading levels, not counting zero. The terminal and SVG use the same number. */
export const LEVELS = 4;

const HTML_ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};

/**
 * Checks the report data and derives the values that never change.
 *
 * @throws When the data is missing.
 */
export function loadReportData(raw: ReportData | undefined): ReportContext {
  if (!raw) throw new Error("git-when data is missing");
  const data: ReportData = {
    ...raw,
    authors: raw.authors || [],
    offsets: raw.offsets || [],
  };
  // The weekend and week start follow --weekend and --week-start.
  const weekend =
    data.meta.weekend && data.meta.weekend.length ? data.meta.weekend : DEFAULT_WEEKEND;
  const weekStart = data.meta.weekStart || 0;
  // An index that does not point to an author falls back to all authors.
  const da = data.meta.defaultAuthor;
  const defaultAuthor =
    typeof da === "number" && Number.isInteger(da) && da >= 0 && da < data.authors.length
      ? da
      : ALL;
  return {
    data,
    weekend,
    weekStart,
    dayOrder: Array.from({ length: 7 }, (_, i) => (weekStart + i) % 7),
    rosterOrder: data.projects
      .map((_, i) => i)
      .sort((a, b) => data.projects[b].commits - data.projects[a].commits || a - b),
    authorTotals: authorTotals(data),
    defaultAuthor,
  };
}

/**
 * Returns the author indexes for the author list, with the most commits first.
 *
 * Authors with fewer commits than --min-total are left out.
 * Their commits still count in every view and total.
 * The default author is always listed, so the list can show the author selected at start.
 */
export function listedAuthors(ctx: ReportContext): number[] {
  const min = ctx.data.meta.minTotal || 0;
  return ctx.data.authors
    .map((_, i) => i)
    .filter((i) => ctx.authorTotals[i] >= min || i === ctx.defaultAuthor)
    .sort((x, y) => ctx.authorTotals[y] - ctx.authorTotals[x] || x - y);
}

/** Reports whether hour is in the night hours 22:00-05:59. Go uses the same rule in stats.IsNight. */
export const isNight = (hour: number): boolean => hour >= 22 || hour < 6;

/**
 * Folds the buckets into the counts that the views need, for the current selection.
 */
export function marginalize(data: ReportData, sel: Selection): Marginals {
  const wh = Array.from({ length: 7 }, () => new Array<number>(24).fill(0));
  const ym = Array.from({ length: data.years.length }, () => new Array<number>(12).fill(0));
  const byProject =
    sel.project < 0 ? Array.from({ length: 7 * 24 }, () => new Map<number, number>()) : null;
  const authors = new Set<number>();
  const projects = new Set<number>();
  const b = data.buckets;
  let total = 0;

  for (let i = 0; i < b.length; i += BUCKET_STRIDE) {
    const a = b[i];
    const p = b[i + 1];
    const y = b[i + 2];
    const mo = b[i + 3];
    const d = b[i + 4];
    const h = b[i + 5];
    const c = b[i + 6];
    if (sel.author >= 0 && a !== sel.author) continue;
    if (sel.project >= 0 && p !== sel.project) continue;
    if (ym[y]) ym[y][mo] += c;
    if (sel.year >= 0 && y !== sel.year) continue;
    wh[d][h] += c;
    total += c;
    authors.add(a);
    projects.add(p);
    if (byProject) {
      const m = byProject[d * 24 + h];
      m.set(p, (m.get(p) || 0) + c);
    }
  }
  return {
    wh,
    ym,
    total,
    byProject,
    authors: authors.size,
    projects: projects.size,
  };
}

/**
 * Computes the peak, totals, weekend index, and night share of `wh`.
 *
 * The weekend index is `share / base`. It is 0 when there are no commits.
 */
export function weekStats(wh: number[][], total: number, weekend: number[]): WeekStats {
  let peak = { d: 0, h: 0, c: 0 };
  let weekendN = 0;
  let nightN = 0;
  const hourTotals = new Array<number>(24).fill(0);
  const dayTotals = new Array<number>(7).fill(0);
  for (let d = 0; d < 7; d++) {
    for (let h = 0; h < 24; h++) {
      const v = wh[d][h];
      if (v > peak.c) peak = { d, h, c: v };
      if (weekend.includes(d)) weekendN += v;
      if (isNight(h)) nightN += v;
      hourTotals[h] += v;
      dayTotals[d] += v;
    }
  }
  const share = total ? weekendN / total : 0;
  const base = weekend.length / 7;
  return {
    peak,
    weekendN,
    nightN,
    share,
    base,
    hourTotals,
    dayTotals,
    index: base ? share / base : 0,
    nightShare: total ? nightN / total : 0,
  };
}

/** Maps a weekend index to one of five summary sentences, from 0 (weekdays only) to 4 (weekend project). */
export function verdictLevel(index: number): number {
  if (index < 0.6) return 0;
  if (index < 0.9) return 1;
  if (index < 1.2) return 2;
  if (index < 1.8) return 3;
  return 4;
}

/** Commits per project with the author and year filters applied. */
export function projectCounts(data: ReportData, sel: Selection): number[] {
  const counts = new Array<number>(data.projects.length).fill(0);
  const b = data.buckets;
  for (let i = 0; i < b.length; i += BUCKET_STRIDE) {
    if (sel.author >= 0 && b[i] !== sel.author) continue;
    if (sel.year >= 0 && b[i + 2] !== sel.year) continue;
    if (b[i + 1] < counts.length) counts[b[i + 1]] += b[i + 6];
  }
  return counts;
}

/**
 * Returns the display name of each author, by author index.
 *
 * A name that only one author has is shown as it is.
 * Authors that share a name show their email.
 * Without emails (git-when --no-email) they are numbered instead, with the most commits as (1).
 */
export function authorLabels(ctx: ReportContext): string[] {
  const { authors } = ctx.data;
  const byName = new Map<string, number[]>();
  authors.forEach((a, i) => byName.set(a.name, [...(byName.get(a.name) || []), i]));
  const labels = authors.map((a) => a.name);
  for (const same of byName.values()) {
    if (same.length < 2) continue;
    same
      .sort((x, y) => ctx.authorTotals[y] - ctx.authorTotals[x] || x - y)
      .forEach((i, n) => {
        const a = authors[i];
        labels[i] = a.email ? `${a.name} <${a.email}>` : `${a.name} (${n + 1})`;
      });
  }
  return labels;
}

/**
 * Returns the index of the first year that has commits in `ym` (`ym[yearIndex][month]`).
 *
 * The month view starts from this year, so years before the first activity of the selected
 * author or project are not drawn as empty rows. Empty years after it stay, so gaps remain visible.
 * Without any commits it returns 0, and every year is drawn.
 */
export function firstActiveYear(ym: number[][]): number {
  const i = ym.findIndex((row) => row.some((v) => v > 0));
  return i < 0 ? 0 : i;
}

/** Builds the time zone table with the author and project filters applied. The year filter is not applied. */
export function offsetTable(data: ReportData, sel: Selection): OffsetTable {
  const raw = data.offsets;
  const offsets: number[] = [];
  const years = new Set<number>();
  const cells = new Map<string, number>();
  for (let i = 0; i < raw.length; i += OFFSET_STRIDE) {
    const a = raw[i];
    const p = raw[i + 1];
    const y = raw[i + 2];
    const o = raw[i + 3];
    const c = raw[i + 4];
    if (sel.author >= 0 && a !== sel.author) continue;
    if (sel.project >= 0 && p !== sel.project) continue;
    years.add(y);
    if (!offsets.includes(o)) offsets.push(o);
    cells.set(`${y}:${o}`, (cells.get(`${y}:${o}`) || 0) + c);
  }
  offsets.sort((a, b) => a - b);
  let max = 0;
  for (const v of cells.values()) max = Math.max(max, v);
  return {
    offsets,
    years: [...years].sort((a, b) => a - b),
    max,
    count: (year, offset) => cells.get(`${year}:${offset}`) || 0,
  };
}

/**
 * Maps count `v` to a shading level from 0 to {@link LEVELS}, relative to `peak`.
 *
 * It uses the same square root scale as Go's render.Scale.Level.
 * One commit is always level 1 and the peak is always the top level.
 */
export function levelOf(v: number, peak: number): number {
  if (v <= 0 || peak <= 0) return 0;
  if (v >= peak) return LEVELS;
  const r = (Math.sqrt(v) - 1) / (Math.sqrt(peak) - 1);
  return 1 + Math.min(Math.floor(r * LEVELS), LEVELS - 1);
}

/** Returns the smallest count for each level, for a legend. Levels that cannot appear are left out. */
export function levelSteps(peak: number): LevelStep[] {
  const out: LevelStep[] = [];
  for (let v = 1, last = 0; v <= peak && last < LEVELS; v++) {
    const l = levelOf(v, peak);
    if (l > last) {
      out.push({ level: l, min: v });
      last = l;
    }
  }
  return out;
}

export const formatNumber = (n: number): string => n.toLocaleString("en-US");

export const percent = (n: number): string => (n * 100).toFixed(1) + "%";

export const pad2 = (n: number): string => String(n).padStart(2, "0");

/** Formats a UTC offset in seconds, such as "+09:00". */
export function formatOffset(sec: number): string {
  const sign = sec < 0 ? "-" : "+";
  const s = Math.abs(sec);
  return `${sign}${pad2(Math.floor(s / 3600))}:${pad2(Math.floor((s % 3600) / 60))}`;
}

/** Escapes text for innerHTML. Pass every user-provided string, such as a repository name, through it. */
export const escapeHtml = (s: unknown): string =>
  String(s).replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch]);

function authorTotals(data: ReportData): number[] {
  const totals = new Array<number>(data.authors.length).fill(0);
  const b = data.buckets;
  for (let i = 0; i < b.length; i += BUCKET_STRIDE) {
    if (b[i] < totals.length) totals[b[i]] += b[i + 6];
  }
  return totals;
}
