# Architecture

This page gives maintainers an overview of the code.
For the behavior that must not change, see [spec.md](spec.md).

## Packages

| Package             | Responsibility                                                                                                                                 |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/git-when`      | The command. Flag parsing (`cli.go`), output selection (`output.go`), run metadata and timings (`report.go`), and the overall run (`main.go`). |
| `internal/discover` | Finds Git repositories under the given directories.                                                                                            |
| `internal/collect`  | Runs `git log`, parses it, filters authors, and adds commits to the aggregate.                                                                 |
| `internal/cache`    | Stores `git log` output on disk.                                                                                                               |
| `internal/model`    | The aggregate data model and its JSON form.                                                                                                    |
| `internal/axis`     | Folds the aggregate onto two axes to make a grid.                                                                                              |
| `internal/stats`    | Weekday and weekend splits, the weekend index, summaries, and UTC offsets per year.                                                            |
| `internal/render`   | Terminal views, CSV, SVG, and the HTML report.                                                                                                 |
| `internal/progress` | The one-line progress display on stderr.                                                                                                       |
| `internal/gittest`  | Test helpers that create repositories with known commit dates.                                                                                 |
| `frontend/`         | The Astro source of the HTML report template.                                                                                                  |

## Data flow

1. `cmd/git-when/cli.go` parses and validates the flags. Invalid flags stop the run before any Git command.
2. `discover` walks the directories and returns the repositories.
3. `collect` reads each repository, using `cache` when possible, and fills one `model.Aggregate`.
4. `output.go` picks the views and the output files.
5. `render` draws the aggregate. `axis` and `stats` compute what each view needs.

Every view reads the same aggregate. History is read only once per run.

## Repository discovery

`discover` walks each root with `filepath.WalkDir`.
A directory is a repository when it has a `.git` directory, a `.git` file with `gitdir:`, or a bare layout.
The walk does not enter a repository or a skipped directory such as `node_modules`.
It enters a symbolic link only with `--follow-symlinks`.
A link whose target is inside the root, or overlaps a directory already searched, is skipped so that the walk cannot loop.
Unreadable directories are reported as warnings and skipped.

## Commit collection and cache

`collect` runs `git log --no-merges --use-mailmap` and reads the author date, name, and email.
The raw output is cached before filtering, so changing `--author` still uses the cache.
The cache key includes the path, HEAD, the log format, and the work tree `.mailmap`.
A broken cache entry is ignored. The cache never changes results.

## Aggregate model

`model.Aggregate` counts commits in buckets.
A bucket key has six fields: author, project, year, month, weekday, and hour.
Keys are packed into a `uint64`, so sorting keys gives a stable order.
A second map counts commits per author, project, year, and recorded UTC offset.
`model/json.go` writes both maps as flat arrays (JSON schema 1).

## Renderers

Terminal views and SVG use `axis.Fold` for grids and `stats` for the other views.
CSV writes every bucket as one row.
The HTML report embeds the aggregate JSON and computes its views in the browser.
All formats write the run metadata, so a file shows how it was made.

## HTML report build

`pnpm build:template` runs `frontend/build-template.mjs`.

1. Astro builds `frontend/src/pages/index.astro`.
2. esbuild compiles `sample.ts` into a plain script.
3. The sample script goes between `<!--@git-when-data-start-->` and `<!--@git-when-data-end-->`.
4. The bundled application script is inlined, and the HTML is minified.
5. The build checks the markers, the script count, and that nothing loads from outside.

The result is committed as `internal/render/template.html` and embedded with `go:embed`.
At run time, `render.HTML` replaces the marked block with a script that holds the real data.

## Frontend code

- `frontend/src/scripts/report/` has code without DOM access: types, data loading, aggregation, formatting, and messages. Vitest tests live here.
- `frontend/src/scripts/ui/` has code that uses the DOM: element lookup, controls, and view drawing.
- `frontend/src/scripts/template.ts` is the entry point. It creates the shared `App` object and draws the page.
- `frontend/src/components/` has one Astro component per page area and per visualization panel. Shared chart CSS is in `ReportViews.astro`.

## Adding a view

Terminal or SVG view:

1. Compute the data in `internal/stats`, or use `axis.Fold` for a grid.
2. Add a renderer in `internal/render` with tests.
3. Add the view name to `parseView`, `termViews`, and `sectionTitle` in `cmd/git-when/output.go`. Add it to `svgViews` only if SVG can draw it.
4. Update the flag help in `cmd/git-when/cli.go` and the view table in [spec.md](spec.md).

HTML view:

1. Add the id to `VIEW_IDS` in `report/types.ts`. The tab is generated from it.
2. Add `tab.*` and `note.*` messages in `report/i18n.ts` for every language.
3. Add a panel component and include it in `ReportViews.astro`.
4. Add the elements to `queryReportElements` and a draw function in `ui/views.ts`.
5. Run `pnpm test:template` and `pnpm build:template`, then commit the template.
