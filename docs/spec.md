# Specification

This page describes the behavior of git-when that users and scripts rely on.
Changes to anything here are user-visible changes.

## Command

```text
git-when [flags] [dir...]
```

If no directories are given, git-when searches the current directory.
Run `git-when --help` to see all flags.
`git-when --version` prints the version embedded in the Go source and exits.

## Repository discovery

- Each directory is searched up to `--max-depth` levels (default 5). `0` means no limit.
- A repository is a directory with a `.git` directory, a `.git` file that starts with `gitdir:`, or a bare layout (`HEAD`, `objects/`, `refs/`).
- The search does not enter a repository, so submodules and nested repositories are not counted twice.
- These directories are skipped: `node_modules`, `bower_components`, `vendor`, `target`, `.venv`, `venv`, `__pycache__`, `.tox`, `.terraform`. A directory with one of these names is still searched when given as a root.
- Symbolic links are not followed unless `--follow-symlinks` is given. A root that is a link is always resolved first.
- With `--follow-symlinks`, a link to a directory is searched and its repositories are named by the link path. The depth includes the levels traversed through the link.
- A followed link is skipped when its target is inside the root, or when it contains or is inside a directory already searched. This stops loops and counts each directory once.
- Repository names are paths relative to the root. With several roots, names start with the base name of the root.
- At most 65536 repositories are supported.

## Commits and authors

- git-when reads non-merge commits reachable from `HEAD` with `.mailmap` applied.
- An author is a unique name and email pair.
- `--author` and `--exclude-author` are regular expressions matched anywhere in `Name <email>`.
- Bot commits are excluded by default. `--bots` includes them. Bots are excluded before `--author` is applied.

## Time

- Every commit is counted in its author's local time. The local time comes from the author date and UTC offset recorded in the commit. No time zone conversion is done.
- Weekdays are numbered 0 for Monday to 6 for Sunday in all internal data and JSON.
- Night hours are 22:00-05:59.
- The weekend is Saturday and Sunday unless `--weekend` sets other days. At least one weekday must remain.
- The weekend index is the weekend share of commits divided by (the number of weekend days / 7). 1.00 means weekend days are as busy per day as weekdays.

## Aggregation

Commits are counted in buckets of author, project, year, month, weekday, and hour.
Commits are also counted per author, project, year, and recorded UTC offset.
All views are computed from these counts.

## Views

| View      | Terminal | SVG | Content                                      |
| --------- | -------- | --- | -------------------------------------------- |
| `heatmap` | yes      | yes | Weekday-by-hour grid (default)               |
| `hour`    | yes      | yes | Commits by hour (`-p /hour`)                 |
| `month`   | yes      | yes | Year by month grid (`-p year/month`)         |
| `week`    | yes      | yes | Weekday and weekend hourly bars on one scale |
| `weekday` | yes      | no  | Commits per weekday                          |
| `days`    | yes      | no  | Hourly bars for each weekday                 |
| `summary` | yes      | no  | One row per repository                       |
| `tzshift` | yes      | no  | UTC offsets per year                         |

- `--view` takes a comma-separated list or `all`. Views are drawn in the given order.
- `--pivot y/x` draws any grid. Axes are `hour`, `wday`, `month`, `year`, `project`, and `author`. The y axis may be empty.
- `--view` and `--pivot` cannot be combined.
- `--min-total` hides grid rows with fewer commits. In the terminal and SVG it works only with grids.
- The HTML report has the views `heatmap`, `weekday-hours`, `week`, `month`, and `tzshift`. Viewers can switch between them on the page.
- In the HTML report, `--min-total` hides authors with fewer commits in total from the author list. Their commits still count in every view and in the total.
- The CSV format does not accept `--view`, `--pivot`, or `--min-total`. The HTML format does not accept `--view` or `--pivot`.
- `--default-author` selects the author whose `Name <email>` matches the regular expression when the HTML page opens. If several authors match, the author with the most commits is selected. The author stays in the author list even if `--min-total` would otherwise hide them.
- `--default-author` works only with HTML. When no author matches, git-when exits with code 1.
- The HTML page has a link to the git-when repository on GitHub in the top bar. `--no-github-link` leaves it out. The option works only with HTML.
- `--no-email` leaves author emails out of the HTML report. Authors are not merged. Authors that share a name are shown as `Name (1)`, `Name (2)`, and so on, with the most commits first. The option works only with HTML. The recorded command line is not changed, so an email given in a flag such as `--author` stays in it.

## Output formats

| Format           | Default output  | Notes                                                                       |
| ---------------- | --------------- | --------------------------------------------------------------------------- |
| `term` (default) | stdout          | Plain text. `--color` shades grid cells with one ANSI color ramp.           |
| `csv`            | `git-when.csv`  | One row per bucket. Leading `#` lines record the run settings.              |
| `svg`            | `git-when.svg`  | Several views in one sheet, or one file per view with `--svg-layout split`. |
| `html`           | `git-when.html` | One offline file with every view.                                           |

- `-o -` writes to stdout. `-o dir/` with SVG writes one file per view named `git-when.all.<view>.svg`.
- Without `-o`, split SVG output is written to the current directory.
- When git-when writes a file, it prints `wrote <path>` on stderr.
- In CSV, `month` is 1-12 and `weekday` is ISO 8601 (1 for Monday to 7 for Sunday).

## Defaults

| Flag                      | Default                                 |
| ------------------------- | --------------------------------------- |
| `--format`                | `term`                                  |
| `--view`                  | `heatmap`                               |
| `--week-start`            | `mon`                                   |
| `--weekend`               | `sat,sun`                               |
| `--scale`                 | `sqrt`                                  |
| `--color`                 | `never`                                 |
| `--theme`                 | `auto`                                  |
| `--progress`, `--timings` | `auto` (only when stderr is a terminal) |
| `--max-depth`             | `5`                                     |
| `--follow-symlinks`       | off                                     |
| `--no-github-link`        | off (the HTML page shows the link)      |
| `--no-email`              | off (the HTML report has emails)        |

## Cache

- `git log` output is cached in the user cache directory under `git-when`.
- `--cache-dir` sets another directory. `--no-cache` disables the cache. The two cannot be combined.
- Repositories without commits are not cached.
- If no cache directory is available, git-when runs without a cache.

## Deterministic output

The same repositories and flags produce byte-for-byte identical output.
Keys are sorted and no timestamps are written.
The recorded command line is part of the output.

## Errors and exit codes

| Code | Meaning                                                                                                    |
| ---- | ---------------------------------------------------------------------------------------------------------- |
| 0    | Success, including `--help`.                                                                               |
| 1    | Runtime error, such as finding no repositories, finding too many repositories, or failing to write a file. |
| 2    | Invalid flags or arguments.                                                                                |

- Errors are printed as `error: <message>` on stderr.
- A repository or directory that cannot be read is skipped with `warning: skipping <path>: <reason>`. It does not change the exit code.
- Files are written only after rendering succeeds, so a failed run leaves no partial files.

## JSON schema

The HTML report embeds the aggregate as JSON with these fields.

| Field      | Content                                                                                                                                                                                                                                                                                                                                                |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `schema`   | Always `1`.                                                                                                                                                                                                                                                                                                                                            |
| `meta`     | `tool`, `version`, `tz` (always `"author"`), `period`, `weekStart`, `weekend`, `minTotal` (only when `--min-total` is given), `defaultAuthor` (author index, only when `--default-author` is given), `noGitHubLink` (only when `--no-github-link` is given), `noEmail` (only when `--no-email` is given), `command`, `roots` (always `[]`), `filters`. |
| `authors`  | `{name, email}` in order of first appearance. With `--no-email`, `email` is `""`.                                                                                                                                                                                                                                                                      |
| `projects` | `{name, path, head, commits}` in discovery order. `path` is always `""`.                                                                                                                                                                                                                                                                               |
| `years`    | Years that appear in the data, ascending.                                                                                                                                                                                                                                                                                                              |
| `buckets`  | Flat array with 7 numbers per record.                                                                                                                                                                                                                                                                                                                  |
| `offsets`  | Flat array with 5 numbers per record.                                                                                                                                                                                                                                                                                                                  |

- A bucket record is `[author, project, yearIndex, month, weekday, hour, count]`.
  `month` is 0-11, `weekday` is 0 for Monday to 6 for Sunday, and `hour` is 0-23.
- An offset record is `[author, project, yearIndex, offsetSeconds, count]`.
- `author` and `project` are indexes into `authors` and `projects`. `yearIndex` is an index into `years`.
- Records are sorted, and empty arrays are written as `[]`.

## HTML data contract

- `internal/render/template.html` has exactly one `<!--@git-when-data-start-->` followed by one `<!--@git-when-data-end-->`.
- The block between them contains exactly one script that sets `window.__GIT_WHEN_DATA__`.
- Go replaces the whole block, markers included, with `<script>window.__GIT_WHEN_DATA__=<json>;</script>`.
- Outside the block, the page has exactly one application script. It loads nothing from the network.
- The only URL in the page is the link to `https://github.com/tsutsu3/git-when`. The page makes no network request unless the viewer follows the link.
- The report contains no local paths because it is often shared or published. Project paths and search roots are empty, and the home directory in the command line is written as `~`. This sanitization does not affect terminal, CSV, or SVG output.
- The report uses English and the system color scheme by default. The viewer can switch the language to Japanese and choose a light or dark theme. These choices are stored only in the browser.
