/**
 * Shared types of the HTML report.
 *
 * The data types mirror the JSON that Go writes for model.Aggregate (schema 1).
 *
 * @module
 */

/** A display language. */
export type Lang = "en" | "ja";

/** A color theme choice. "system" follows the operating system setting. */
export type ThemePreference = "light" | "dark" | "system";

/** The id of one visualization. */
export type ViewId = (typeof VIEW_IDS)[number];

/** One commit author. */
export interface Author {
  name: string;
  /** Empty with git-when --no-email. */
  email: string;
}

/** One repository. */
export interface Project {
  name: string;
  path: string;
  /** HEAD at analysis time. Empty when the repository has no commits. */
  head: string;
  commits: number;
}

/** How the report was generated. */
export interface Meta {
  tool: string;
  version: string;
  /** The time basis. Always "author" (author local time). */
  tz: string;
  /** The first and last month, such as "2023-01 to 2026-09". */
  period: string;
  /** The first day of the week, 0=Mon..6=Sun. */
  weekStart: number;
  /** The weekend days, 0=Mon..6=Sun. */
  weekend: number[];
  /** --min-total. Authors with fewer commits are left out of the author list. Missing means 0. */
  minTotal?: number;
  /** --default-author. The author index selected when the page opens. Missing means all authors. */
  defaultAuthor?: number;
  /** --no-github-link. When true, the top bar has no link to the git-when repository. */
  noGitHubLink?: boolean;
  /** --no-email. Author emails are empty, and authors with the same name are numbered. */
  noEmail?: boolean;
  command: string;
}

/**
 * The report data (JSON schema 1).
 *
 * `buckets` and `offsets` are flat arrays.
 * Each bucket is `[author, project, yearIndex, month 0-11, weekday 0=Mon..6=Sun, hour, count]`.
 * Each offset record is `[author, project, yearIndex, offsetSeconds, count]`.
 * `yearIndex` is an index into `years`.
 */
export interface ReportData {
  schema: number;
  authors: Author[];
  projects: Project[];
  years: number[];
  buckets: number[];
  offsets: number[];
  meta: Meta;
}

/** What the viewer has selected. Each index is {@link ALL} when not filtered. */
export interface Selection {
  /** Index into `projects`. */
  project: number;
  /** Index into `authors`. */
  author: number;
  /** Index into `years`. */
  year: number;
  view: ViewId;
  /** Text typed into the repository filter. */
  rosterQuery: string;
}

/** The visualizations, in tab order. */
export const VIEW_IDS = ["heatmap", "weekday-hours", "week", "month", "tzshift"] as const;

/** The selection value that means "not filtered". */
export const ALL = -1;

/** Reports whether value is a known view id. */
export function isViewId(value: unknown): value is ViewId {
  return (VIEW_IDS as readonly unknown[]).includes(value);
}
