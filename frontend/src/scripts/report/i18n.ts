/**
 * Messages of the HTML report and the translate function.
 *
 * Every visible text comes from here. `{name}` placeholders are filled from parameters.
 * Messages that go into innerHTML must not receive user data unless it is escaped with escapeHtml.
 *
 * @module
 */

import type { Lang } from "./types";

/** The key of a message. */
export type MessageKey = keyof typeof en;

/** A message table, possibly incomplete. */
export type MessageTable = Record<Lang, Partial<Record<string, string>>>;

/** Placeholder values for a message. */
export type MessageParams = Record<string, string | number>;

/** English month abbreviations. Japanese uses month numbers. */
export const MONTH_NAMES_EN = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
] as const;

const en = {
  projects: "Repositories",
  filterPlaceholder: "Filter by name",
  filterLabel: "Filter repositories by name",
  openProjects: "Open the repository list",
  hideProjects: "Close the repository list",
  author: "Author",
  period: "Period",
  reset: "Clear filters",
  langGroup: "Language",
  themeGroup: "Theme",
  "theme.light": "Light",
  "theme.dark": "Dark",
  "theme.system": "Match system",
  overview: "Overview",
  viewsLabel: "Views",
  "tab.heatmap": "Weekday × hour",
  "tab.weekday-hours": "Hours by weekday",
  "tab.week": "Weekdays vs weekend",
  "tab.month": "By month",
  "tab.tzshift": "Time zones",
  "note.heatmap":
    "See where activity gathers across the week, using the local time recorded in each commit.",
  "note.weekday-hours":
    "Each day has its own rhythm. Compare them side by side to see how activity shifts through the week.",
  "note.week":
    "See whether activity stays within the working week or spills into the weekend, hour by hour.",
  "note.month":
    "Follow the project’s pace over time and spot busy periods, quiet stretches, and longer-term shifts.",
  "note.tzshift":
    "Follow changes in the UTC offsets recorded in commits. They can hint at travel, relocation, or a changing place of work.",
  runInfo: "How this report was generated",
  allProjects: "All repositories",
  allAuthors: "All authors ({n})",
  anyAuthor: "All authors",
  authorOption: "{name} · {n} commits",
  githubLink: "git-when on GitHub",
  hiddenAuthors: "{n} authors with fewer than {min} commits are hidden",
  allTime: "All time",
  year: "{y}",
  localTime: "local time recorded in commits",
  noProjectMatch: "No matching repositories",
  noCommits: "No commits match these filters. Try clearing the author or period.",
  lede: "<b>{when}</b> is the busiest time, and {verdict}.",
  ledeWhen: "{day} {h}:00",
  "verdict.0": "the weekend is barely touched",
  "verdict.1": "weekends are quiet",
  "verdict.2": "there is no real weekday/weekend skew",
  "verdict.3": "weekends are busier",
  "verdict.4": "this is a weekend project",
  "kpi.commits": "Commits",
  "kpi.commitsSub": "{p} repositories · {a} authors",
  "kpi.peak": "Busiest slot",
  "kpi.index": "Weekend index",
  "kpi.indexSub": "{share} on weekends (even by day: {base})",
  "kpi.night": "Night 22:00–5:59",
  "kpi.none": "none",
  commits: "{n} commits",
  total: "Total",
  hourSlot: "{h}:00–{h}:59",
  dayHour: "{day} {h}:00",
  peakAt: "busiest: {where} ({n})",
  noCommitsShort: "no commits",
  dayHead: "{day}",
  dayHeadWeekend: "{day} (weekend)",
  dayPeak: "{n} · peak {h}:00",
  weekdaysHead: "Weekdays ({n} days)",
  weekendHead: "Weekend: {days} ({n} days)",
  daySep: ", ",
  indexLabel: "Weekend index",
  indexText:
    "An index of 1.00 means the same average activity per day on weekdays and weekends. Higher values lean toward weekends; lower values toward weekdays.",
  gaugeEven: "1.00 even",
  yearMonth: "{mon} {y}",
  yearOffset: "{y} {o}",
  most: "Top",
  maxCommits: "max {n} commits",
  noData: "No data matches these filters.",
  "tz.author": "local time recorded in commits",
  "tz.utc": "UTC",
  "tz.system": "system",
  runInfoLine: "Time: {tz} · Period: {period} · Repositories: {n}",
};

const ja: Record<MessageKey, string> = {
  projects: "プロジェクト",
  filterPlaceholder: "名前で絞り込む",
  filterLabel: "プロジェクトを名前で絞り込む",
  openProjects: "プロジェクト一覧を開く",
  hideProjects: "プロジェクト一覧を閉じる",
  author: "コミット作成者",
  period: "期間",
  reset: "条件をクリア",
  langGroup: "言語",
  themeGroup: "配色",
  "theme.light": "ライト",
  "theme.dark": "ダーク",
  "theme.system": "システムに合わせる",
  overview: "概要",
  viewsLabel: "ビュー",
  "tab.heatmap": "曜日 × 時間",
  "tab.weekday-hours": "曜日ごとの時間分布",
  "tab.week": "平日 vs 週末",
  "tab.month": "月別",
  "tab.tzshift": "タイムゾーン",
  "note.heatmap":
    "一週間のどこに活動が集まっているかを、コミットに記録されたローカル時刻で俯瞰できます。",
  "note.weekday-hours":
    "曜日ごとのリズムを並べると、一週間を通して活動時間がどう変わるかが見えてきます。",
  "note.week": "活動が平日に収まっているか、週末にも広がっているかを、時間帯ごとに見られます。",
  "note.month":
    "活発だった時期と落ち着いていた時期をたどり、プロジェクトのペースの変化を見渡せます。",
  "note.tzshift":
    "コミットに記録されたUTCオフセットの移り変わりから、活動拠点の変化をたどれます。移動や転居が表れている場合があります。",
  runInfo: "生成条件と対象リポジトリ",
  allProjects: "すべてのプロジェクト",
  allAuthors: "すべての作成者（{n}人）",
  anyAuthor: "すべての作成者",
  authorOption: "{name} · {n}件",
  githubLink: "GitHub の git-when",
  hiddenAuthors: "コミット{min}件未満の{n}人は非表示",
  allTime: "全期間",
  year: "{y}年",
  localTime: "コミットに記録されたローカル時刻",
  noProjectMatch: "該当するプロジェクトはありません",
  noCommits: "この条件に該当するコミットはありません。作成者や期間の指定を外してみてください。",
  lede: "<b>{when}</b>が最も忙しく、{verdict}。",
  ledeWhen: "{day}曜の {h}時",
  "verdict.0": "週末はほとんど触っていません",
  "verdict.1": "週末は控えめです",
  "verdict.2": "曜日による偏りはほぼありません",
  "verdict.3": "週末のほうが動いています",
  "verdict.4": "週末のプロジェクトです",
  "kpi.commits": "コミット",
  "kpi.commitsSub": "{p} リポジトリ · {a} 人",
  "kpi.peak": "最も多い時間帯",
  "kpi.index": "週末指数",
  "kpi.indexSub": "週末 {share}（曜日が均等なら {base}）",
  "kpi.night": "夜間 22:00–5:59",
  "kpi.none": "該当なし",
  commits: "{n} commits",
  total: "計",
  hourSlot: "{h}:00 台",
  dayHour: "{day}曜 {h}:00",
  peakAt: "最も多いのは {where}（{n}）",
  noCommitsShort: "コミットなし",
  dayHead: "{day}曜",
  dayHeadWeekend: "{day}曜（週末）",
  dayPeak: "{n} · ピーク {h}時",
  weekdaysHead: "平日（{n}日）",
  weekendHead: "週末: {days}（{n}日）",
  daySep: "・",
  indexLabel: "週末指数",
  indexText:
    "1.00 は、1日あたりの活動量が平日と週末で同じ状態です。高いほど週末寄り、低いほど平日寄りです。",
  gaugeEven: "1.00 均等",
  yearMonth: "{y}年{m}月",
  yearOffset: "{y}年 {o}",
  most: "最多",
  maxCommits: "最大 {n} commits",
  noData: "この条件に該当するデータはありません。",
  "tz.author": "コミットに記録されたローカル時刻",
  "tz.utc": "UTC",
  "tz.system": "システム",
  runInfoLine: "時刻: {tz} · 期間: {period} · リポジトリ: {n}",
};

/** Messages of every language. The Japanese table must have every English key. */
export const MESSAGES: Record<Lang, Record<MessageKey, string>> = { en, ja };

const DAY_NAMES: Record<Lang, readonly string[]> = {
  en: ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"],
  ja: ["月", "火", "水", "木", "金", "土", "日"],
};

/**
 * Returns the message for `key` in `lang`.
 *
 * A missing message falls back to English, then to the key itself.
 * Missing placeholder values become empty strings.
 */
export function translate(
  lang: Lang,
  key: string,
  params?: MessageParams,
  messages: MessageTable = MESSAGES,
): string {
  const s = messages[lang][key] || messages.en[key] || key;
  return params
    ? s.replace(/\{(\w+)\}/g, (_, k: string) => (params[k] != null ? String(params[k]) : ""))
    : s;
}

/** Reports whether value is a supported language. */
export function isLang(value: unknown): value is Lang {
  return value === "en" || value === "ja";
}

/** Returns the short name of weekday `d` (0=Mon..6=Sun). */
export const dayName = (lang: Lang, d: number): string => DAY_NAMES[lang][d];
