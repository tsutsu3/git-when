# HTML template frontend

This directory holds the Astro source of the HTML report. The Go binary embeds
only the generated file `internal/render/template.html`.

## Layout

```text
build-template.mjs          Astro build, sample data, inlining, minify, and checks
src/pages/index.astro       document shell and page-level responsive CSS
src/components/             TopBar, Sidebar, ReportSummary, ReportViews, the
                            view panels, RunInfo, and Overlay, each with its CSS
src/styles/global.css       color tokens, reset, and shared utilities
src/scripts/template.ts     entry point that loads data, sets up controls, and renders
src/scripts/sample.ts       deterministic sample data
src/scripts/report/         no DOM access
  types.ts                  data types, Lang, ThemePreference, ViewId, VIEW_IDS
  data.ts                   aggregation, shading levels, and formatting
  i18n.ts                   en and ja messages
  *.test.ts                 Vitest unit tests
src/scripts/ui/             DOM code
  dom.ts                    element lookup, tooltip, and legend helpers
  controls.ts               language, theme, sidebar, filters, tabs, localStorage
  views.ts                  summary, KPIs, and the five views
```

Component CSS uses `<style is:global>`. The report DOM is built at run time, so
Astro scoped attributes would not match it.

## Sample data

The page reads its data from `window.__GIT_WHEN_DATA__`.

- **Development.** With `pnpm dev:template`, `index.astro` loads `sample.ts`
  as a module before `template.ts`.
- **Generated template.** The build replaces `<!--@git-when-sample-->` with the
  compiled sample script between the `@git-when-data-start` and
  `@git-when-data-end` markers. Opening `internal/render/template.html`
  directly in a browser therefore shows the sample.
- **Go.** `git-when --format html` replaces that marker block with real data.

## Commands

Run these from the repository root after `pnpm install --frozen-lockfile`.

```sh
pnpm dev:template    # Astro dev server with sample data
pnpm test:template   # Vitest unit tests for scripts/report
pnpm lint:template   # ESLint, declaration order only
pnpm build:template  # tsc --noEmit, then write internal/render/template.html
pnpm check:template  # fail if the committed template is stale
pnpm format          # Prettier
```

The generated report has no runtime dependencies and works from `file://`
without a network connection.

See [architecture.md](../docs/architecture.md) for the build steps and how to
add a view, and [spec.md](../docs/spec.md) for the data contract.
