import { describe, expect, it } from "vitest";

import {
  authorLabels,
  escapeHtml,
  firstActiveYear,
  formatOffset,
  levelOf,
  levelSteps,
  listedAuthors,
  loadReportData,
  marginalize,
  offsetTable,
  projectCounts,
  verdictLevel,
  weekStats,
} from "./data";
import { ALL, type ReportData, type Selection } from "./types";

// Two authors, two projects, and two years.
const data: ReportData = {
  schema: 1,
  authors: [
    { name: "Alice", email: "alice@example.com" },
    { name: "Bob", email: "bob@example.com" },
  ],
  projects: [
    { name: "app", path: "", head: "", commits: 4 },
    { name: "lib", path: "", head: "", commits: 3 },
  ],
  years: [2024, 2025],
  // author, project, yearIndex, month, weekday, hour, count
  buckets: [
    [0, 0, 0, 2, 1, 9, 2], // Alice, app, 2024-03, Tue 09
    [0, 0, 1, 0, 5, 23, 2], // Alice, app, 2025-01, Sat 23
    [1, 1, 0, 2, 0, 10, 1], // Bob, lib, 2024-03, Mon 10
    [1, 1, 1, 5, 6, 3, 2], // Bob, lib, 2025-06, Sun 03
  ].flat(),
  // author, project, yearIndex, offsetSeconds, count
  offsets: [
    [0, 0, 0, 32400, 2],
    [0, 0, 1, -28800, 2],
    [1, 1, 0, 32400, 1],
    [1, 1, 1, 19800, 2],
  ].flat(),
  meta: {
    tool: "git-when",
    version: "test",
    tz: "author",
    period: "2024-03 to 2025-06",
    weekStart: 0,
    weekend: [5, 6],
    command: "git-when .",
  },
};

const all: Selection = {
  project: ALL,
  author: ALL,
  year: ALL,
  view: "heatmap",
  rosterQuery: "",
};

describe("loadReportData", () => {
  it("rejects missing data", () => {
    expect(() => loadReportData(undefined)).toThrow("git-when data is missing");
  });

  it("derives the order and totals", () => {
    const ctx = loadReportData(data);
    expect(ctx.weekend).toEqual([5, 6]);
    expect(ctx.dayOrder).toEqual([0, 1, 2, 3, 4, 5, 6]);
    expect(ctx.rosterOrder).toEqual([0, 1]);
    expect(ctx.authorTotals).toEqual([4, 3]);
  });

  it("follows the week start and falls back to a Saturday and Sunday weekend", () => {
    const ctx = loadReportData({
      ...data,
      meta: { ...data.meta, weekStart: 6, weekend: [] },
    });
    expect(ctx.weekend).toEqual([5, 6]);
    expect(ctx.dayOrder).toEqual([6, 0, 1, 2, 3, 4, 5]);
  });

  it("selects the default author only when the index points to an author", () => {
    const withDefault = (defaultAuthor?: number) =>
      loadReportData({ ...data, meta: { ...data.meta, defaultAuthor } }).defaultAuthor;
    expect(withDefault(1)).toBe(1);
    expect(withDefault(0)).toBe(0);
    expect(withDefault(undefined)).toBe(ALL);
    expect(withDefault(2)).toBe(ALL);
    expect(withDefault(-1)).toBe(ALL);
  });
});

describe("listedAuthors", () => {
  it("lists every author with the most commits first", () => {
    expect(listedAuthors(loadReportData(data))).toEqual([0, 1]);
  });

  it("leaves out authors under minTotal but keeps their commits", () => {
    const ctx = loadReportData({ ...data, meta: { ...data.meta, minTotal: 4 } });
    // Bob has 3 commits and Alice has 4.
    expect(listedAuthors(ctx)).toEqual([0]);
    expect(marginalize(ctx.data, all).total).toBe(7);
  });

  it("keeps the default author even under minTotal", () => {
    const ctx = loadReportData({ ...data, meta: { ...data.meta, minTotal: 4, defaultAuthor: 1 } });
    expect(listedAuthors(ctx)).toEqual([0, 1]);
  });
});

describe("authorLabels", () => {
  // Alice with the old email has 1 commit, Bob has 1, and Alice with the new email has 2.
  const twoAlices = (withEmail: boolean): ReportData => ({
    ...data,
    authors: [
      { name: "Alice", email: withEmail ? "old@example.com" : "" },
      { name: "Bob", email: withEmail ? "bob@example.com" : "" },
      { name: "Alice", email: withEmail ? "new@example.com" : "" },
    ],
    buckets: [
      [0, 0, 0, 2, 1, 9, 1],
      [1, 0, 0, 2, 1, 9, 1],
      [2, 0, 0, 2, 1, 10, 2],
    ].flat(),
  });

  it("shows the email only for a name that several authors share", () => {
    expect(authorLabels(loadReportData(twoAlices(true)))).toEqual([
      "Alice <old@example.com>",
      "Bob",
      "Alice <new@example.com>",
    ]);
  });

  it("numbers authors that share a name by commits when there are no emails", () => {
    expect(authorLabels(loadReportData(twoAlices(false)))).toEqual([
      "Alice (2)",
      "Bob",
      "Alice (1)",
    ]);
  });
});

describe("firstActiveYear", () => {
  const empty = () => new Array<number>(12).fill(0);
  const active = () => [1, ...new Array<number>(11).fill(0)];

  it("skips only the empty years before the first commit", () => {
    // Empty years in the middle and at the end stay.
    expect(firstActiveYear([empty(), empty(), active(), empty(), active(), empty()])).toBe(2);
    expect(firstActiveYear([active(), empty()])).toBe(0);
  });

  it("keeps every year without commits", () => {
    expect(firstActiveYear([empty(), empty()])).toBe(0);
    expect(firstActiveYear([])).toBe(0);
  });
});

describe("marginalize", () => {
  it("counts everything without filters", () => {
    const m = marginalize(data, all);
    expect(m.total).toBe(7);
    expect(m.wh[1][9]).toBe(2);
    expect(m.wh[6][3]).toBe(2);
    expect(m.authors).toBe(2);
    expect(m.projects).toBe(2);
    expect(m.byProject?.[1 * 24 + 9].get(0)).toBe(2);
  });

  it("filters by author and project", () => {
    expect(marginalize(data, { ...all, author: 1 }).total).toBe(3);
    const m = marginalize(data, { ...all, project: 0 });
    expect(m.total).toBe(4);
    expect(m.byProject).toBeNull();
  });

  it("keeps every year in ym when a year is selected", () => {
    const m = marginalize(data, { ...all, year: 0 });
    expect(m.total).toBe(3);
    expect(m.ym[0][2]).toBe(3);
    expect(m.ym[1][0]).toBe(2);
    expect(m.ym[1][5]).toBe(2);
  });
});

describe("weekStats", () => {
  it("computes the weekend index and night share", () => {
    const m = marginalize(data, all);
    const st = weekStats(m.wh, m.total, [5, 6]);
    expect(st.peak).toEqual({ d: 1, h: 9, c: 2 });
    expect(st.weekendN).toBe(4);
    expect(st.share).toBeCloseTo(4 / 7);
    expect(st.base).toBeCloseTo(2 / 7);
    expect(st.index).toBeCloseTo(2);
    expect(st.nightN).toBe(4);
    expect(st.nightShare).toBeCloseTo(4 / 7);
    expect(st.hourTotals[9]).toBe(2);
    expect(st.dayTotals[6]).toBe(2);
  });

  it("uses the weekend definition as the baseline", () => {
    const m = marginalize(data, all);
    const st = weekStats(m.wh, m.total, [4, 5]);
    expect(st.weekendN).toBe(2);
    expect(st.index).toBeCloseTo(2 / 7 / (2 / 7));
  });

  it("returns zeros without commits", () => {
    const empty = Array.from({ length: 7 }, () => new Array<number>(24).fill(0));
    const st = weekStats(empty, 0, [5, 6]);
    expect(st.index).toBe(0);
    expect(st.share).toBe(0);
    expect(st.nightShare).toBe(0);
    expect(st.peak.c).toBe(0);
  });
});

describe("verdictLevel", () => {
  it("maps index ranges to sentences", () => {
    expect([0, 0.59, 0.6, 0.89, 0.9, 1.19, 1.2, 1.79, 1.8, 5].map(verdictLevel)).toEqual([
      0, 0, 1, 1, 2, 2, 3, 3, 4, 4,
    ]);
  });
});

describe("projectCounts", () => {
  it("applies the author and year filters", () => {
    expect(projectCounts(data, all)).toEqual([4, 3]);
    expect(projectCounts(data, { ...all, year: 1 })).toEqual([2, 2]);
    expect(projectCounts(data, { ...all, author: 0 })).toEqual([4, 0]);
  });
});

describe("offsetTable", () => {
  it("builds a year by offset table", () => {
    const table = offsetTable(data, all);
    expect(table.offsets).toEqual([-28800, 19800, 32400]);
    expect(table.years).toEqual([0, 1]);
    expect(table.count(0, 32400)).toBe(3);
    expect(table.count(1, 32400)).toBe(0);
    expect(table.max).toBe(3);
  });

  it("applies the author filter", () => {
    const table = offsetTable(data, { ...all, author: 0 });
    expect(table.offsets).toEqual([-28800, 32400]);
    expect(table.max).toBe(2);
  });
});

describe("levelOf and levelSteps", () => {
  // Expected values come from Go's render.Sqrt.Level and render.Sqrt.Steps.
  it.each([
    [1, [0, 4]],
    [2, [0, 1, 4]],
    [3, [0, 1, 3, 4]],
    [4, [0, 1, 2, 3, 4]],
    [7, [0, 1, 2, 2, 3, 4, 4, 4]],
    [10, [0, 1, 1, 2, 2, 3, 3, 4, 4, 4, 4]],
  ])("matches Go for peak %i", (peak, levels) => {
    expect(Array.from({ length: peak + 1 }, (_, v) => levelOf(v, peak))).toEqual(levels);
  });

  it("matches Go for large peaks", () => {
    const at = (peak: number) =>
      [0, 1, 2, Math.floor(peak / 4), Math.floor(peak / 2), peak - 1, peak].map((v) =>
        levelOf(v, peak),
      );
    expect(at(100)).toEqual([0, 1, 1, 2, 3, 4, 4]);
    expect(at(1000)).toEqual([0, 1, 1, 2, 3, 4, 4]);
  });

  it.each([
    [1, [[4, 1]]],
    [
      3,
      [
        [1, 1],
        [3, 2],
        [4, 3],
      ],
    ],
    [
      7,
      [
        [1, 1],
        [2, 2],
        [3, 4],
        [4, 5],
      ],
    ],
    [
      100,
      [
        [1, 1],
        [2, 11],
        [3, 31],
        [4, 61],
      ],
    ],
    [
      1000,
      [
        [1, 1],
        [2, 75],
        [3, 267],
        [4, 575],
      ],
    ],
  ])("returns the legend steps of Go for peak %i", (peak, steps) => {
    expect(levelSteps(peak)).toEqual(steps.map(([level, min]) => ({ level, min })));
  });

  it("returns no level for zero", () => {
    expect(levelOf(0, 0)).toBe(0);
    expect(levelSteps(0)).toEqual([]);
  });
});

describe("formatting", () => {
  it.each([
    [32400, "+09:00"],
    [-28800, "-08:00"],
    [19800, "+05:30"],
    [-12600, "-03:30"],
    [0, "+00:00"],
  ])("formats offset %i as %s", (sec, want) => {
    expect(formatOffset(sec)).toBe(want);
  });

  it("escapes HTML", () => {
    expect(escapeHtml(`<a href="x">'&'</a>`)).toBe(
      "&lt;a href=&quot;x&quot;&gt;&#39;&amp;&#39;&lt;/a&gt;",
    );
  });
});
