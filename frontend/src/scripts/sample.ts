/**
 * Deterministic sample data for the report template.
 *
 * During `astro dev`, index.astro loads this module before the application script.
 * The template build compiles it into the data script between the Go data markers,
 * so the generated template shows this data when it is opened directly.
 * Go replaces that data script with the real data.
 *
 * The data comes from a seeded random generator, so every build is the same.
 * Each commit picks an author, a weekday, and an hour from weighted profiles.
 * This keeps quiet hours empty and gives every view a visible story.
 *
 * @module
 */

import type { ReportData } from "./report/types";

/** When someone works, as relative weights. */
interface Profile {
  /** Weight of each weekday, 0=Mon..6=Sun. */
  days: number[];
  weekdayHours: number[];
  weekendHours: number[];
}

/** One author's part in a project. */
interface Member {
  author: number;
  share: number;
  profile: Profile;
  /** First active month index. */
  from?: number;
}

/** A sample project and its activity over time. */
interface Shape {
  name: string;
  head: string;
  /** Commits in an average month. */
  base: number;
  /** Relative activity in month index m, where 0 is 2023-01. */
  trend: (m: number) => number;
  /** Relative activity in each calendar month. */
  season: number[];
  team: Member[];
}

const DEFAULT_WEEKEND = [5, 6];

const YEARS = [2023, 2024, 2025, 2026];

/** Number of months from 2023-01 to 2026-09. */
const MONTHS = 45;

const ALICE = 0;

const BOB = 1;

const CAROL = 2;

const DAVE = 3;

const OFFICE: Profile = {
  days: [1, 1, 1, 1, 0.85, 0.05, 0.03],
  weekdayHours: hours(
    (h) =>
      (bump(h, 10.5, 1.4) + 0.9 * bump(h, 15, 1.7) + 0.25 * bump(h, 18.5, 1.2)) *
      (h === 12 ? 0.45 : 1),
  ),
  weekendHours: hours((h) => bump(h, 14, 2.5)),
};

const LATE_OFFICE: Profile = {
  days: OFFICE.days,
  weekdayHours: hours((h) => bump(h, 13.5, 1.6) + bump(h, 17.5, 1.8) + 0.4 * bump(h, 21.5, 1.2)),
  weekendHours: hours((h) => bump(h, 16, 2.5)),
};

const HOBBY: Profile = {
  days: [0.55, 0.5, 0.55, 0.5, 0.7, 1.7, 1.5],
  weekdayHours: hours((h) => bump(h, 21.5, 1.4) + 0.35 * bump(h, 7, 0.8)),
  weekendHours: hours((h) => bump(h, 11, 2) + bump(h, 15.5, 2.2) + 0.6 * bump(h, 22, 1.3)),
};

const NOTES: Profile = {
  days: [1, 1, 1, 1, 0.9, 0.5, 0.5],
  weekdayHours: hours((h) => bump(h, 7.5, 0.7) + 0.3 * bump(h, 12.5, 0.5)),
  weekendHours: hours((h) => bump(h, 9.5, 1.2)),
};

// Office work slows down in Golden Week, Obon, and the new year holidays.
const WORK_SEASON = [0.8, 1, 1.1, 1, 0.7, 1, 1, 0.45, 1, 1.05, 1.1, 0.4];

// Hobby work picks up in the same holidays.
const HOBBY_SEASON = [1.1, 0.9, 0.9, 0.9, 1.2, 0.9, 1, 1.4, 0.9, 0.9, 1, 1.35];

const FLAT_SEASON = Array<number>(12).fill(1);

const SHAPES: Shape[] = [
  {
    name: "work/api-server",
    head: "a3f19c4e8b21d70c",
    base: 60,
    // Grows from a small start, with release crunches in 2024-03 and 2025-11.
    trend: (m) =>
      0.35 +
      (0.8 * Math.min(m, 30)) / 30 -
      (m >= 36 ? 0.3 : 0) +
      (m === 14 ? 0.5 : 0) +
      (m === 34 ? 0.4 : 0),
    season: WORK_SEASON,
    team: [
      { author: ALICE, share: 0.65, profile: OFFICE },
      { author: DAVE, share: 0.35, profile: LATE_OFFICE, from: 18 },
    ],
  },
  {
    name: "work/frontend",
    head: "7d02b5619ac4e883",
    base: 85,
    // Starts in 2024-03 and ramps up. A redesign in 2025-07..09 adds work.
    trend: (m) => (m < 14 ? 0 : Math.min(1, (m - 13) / 4) * (m >= 30 && m <= 32 ? 1.3 : 1)),
    season: WORK_SEASON,
    team: [
      { author: BOB, share: 0.75, profile: OFFICE },
      { author: DAVE, share: 0.25, profile: LATE_OFFICE, from: 18 },
    ],
  },
  {
    name: "work/infra",
    head: "c81e4fa07b3d5926",
    base: 38,
    // Quiet at first, a migration in 2023-09..11, then maintenance only from 2026.
    trend: (m) => (m >= 8 && m <= 10 ? 1.8 : m < 8 ? 0.4 : m <= 35 ? 0.9 : 0.25),
    season: WORK_SEASON,
    team: [{ author: ALICE, share: 1, profile: OFFICE }],
  },
  {
    name: "oss/git-when",
    head: "f45a9e2c77b10d38",
    base: 55,
    // Starts in 2025-02 with a launch burst.
    trend: (m) => (m < 25 ? 0 : m < 28 ? 1.7 : 0.75),
    season: HOBBY_SEASON,
    team: [{ author: CAROL, share: 1, profile: HOBBY }],
  },
  {
    name: "notes/til",
    head: "2b6c0d85f9a3e417",
    base: 14,
    trend: () => 1,
    season: FLAT_SEASON,
    team: [
      { author: CAROL, share: 0.6, profile: NOTES },
      { author: BOB, share: 0.4, profile: NOTES },
    ],
  },
];

function generateSample(): ReportData {
  // A linear congruential generator with a fixed seed.
  const rnd = (
    (s: number) => () =>
      (s = (s * 1664525 + 1013904223) >>> 0) / 4294967296
  )(20260913);

  const pick = (weights: number[]): number => {
    let r = rnd() * weights.reduce((a, b) => a + b, 0);
    for (let i = 0; i < weights.length - 1; i++) {
      r -= weights[i];
      if (r < 0) return i;
    }
    return weights.length - 1;
  };

  // The UTC offset each author recorded, in seconds.
  const offsetOf = (author: number, m: number): number => {
    switch (author) {
      case CAROL: // Moved to California for 2025-04..12.
        if (m >= 27 && m <= 33) return -7 * 3600;
        if (m >= 34 && m <= 35) return -8 * 3600;
        break;
      case BOB: // A business trip to Europe in 2024-10.
        if (m === 21 && rnd() < 0.45) return 2 * 3600;
        break;
      case DAVE: // Works from India.
        return 5.5 * 3600;
    }
    return 9 * 3600;
  };

  // Keys sort in the same order as the Go output.
  const buckets = new Map<number, number>();
  const offsets = new Map<string, number>();
  const commits = SHAPES.map(() => 0);

  for (let m = 0; m < MONTHS; m++) {
    const yi = Math.floor(m / 12);
    const mo = m % 12;
    SHAPES.forEach((s, pi) => {
      const team = s.team.filter((t) => (t.from ?? 0) <= m);
      const expected = s.base * s.trend(m) * s.season[mo];
      const n = Math.round(expected * (0.85 + 0.3 * rnd()));
      for (let i = 0; i < n; i++) {
        const member = team[pick(team.map((t) => t.share))];
        const p = member.profile;
        const d = pick(p.days);
        const h = pick(DEFAULT_WEEKEND.includes(d) ? p.weekendHours : p.weekdayHours);
        const a = member.author;
        const key = ((((a * 8 + pi) * 8 + yi) * 12 + mo) * 7 + d) * 24 + h;
        buckets.set(key, (buckets.get(key) ?? 0) + 1);
        const okey = [a, pi, yi, offsetOf(a, m)].join(",");
        offsets.set(okey, (offsets.get(okey) ?? 0) + 1);
        commits[pi]++;
      }
    });
  }

  const flatBuckets: number[] = [];
  for (const key of [...buckets.keys()].sort((x, y) => x - y)) {
    let k = key;
    const h = k % 24;
    k = Math.floor(k / 24);
    const d = k % 7;
    k = Math.floor(k / 7);
    const mo = k % 12;
    k = Math.floor(k / 12);
    const yi = k % 8;
    k = Math.floor(k / 8);
    const pi = k % 8;
    const a = Math.floor(k / 8);
    flatBuckets.push(a, pi, yi, mo, d, h, buckets.get(key) ?? 0);
  }

  const flatOffsets: number[] = [];
  const offsetRows = [...offsets].map(([k, c]) => [...k.split(",").map(Number), c]);
  offsetRows.sort((x, y) => x[0] - y[0] || x[1] - y[1] || x[2] - y[2] || x[3] - y[3]);
  for (const row of offsetRows) flatOffsets.push(...row);

  return {
    schema: 1,
    authors: [
      { name: "Alice", email: "alice@example.com" },
      { name: "Bob", email: "bob@example.com" },
      { name: "Carol", email: "carol@example.com" },
      { name: "Dave", email: "dave@example.com" },
    ],
    projects: SHAPES.map((s, pi) => ({
      name: s.name,
      path: "",
      head: s.head,
      commits: commits[pi],
    })),
    years: YEARS,
    buckets: flatBuckets,
    offsets: flatOffsets,
    meta: {
      tool: "git-when",
      version: "sample",
      tz: "author",
      period: "2023-01 to 2026-09",
      weekStart: 0,
      weekend: DEFAULT_WEEKEND,
      command: "git-when ~/src -f html -o report.html",
    },
  };
}

function hours(f: (h: number) => number): number[] {
  return Array.from({ length: 24 }, (_, h) => Math.max(f(h), 0.003));
}

/** A bell curve over the hours of a day that wraps at midnight. */
function bump(h: number, mean: number, sd: number): number {
  const d = Math.min(Math.abs(h - mean), 24 - Math.abs(h - mean));
  return Math.exp(-0.5 * (d / sd) ** 2);
}

window.__GIT_WHEN_DATA__ = generateSample();
