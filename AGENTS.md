# AGENTS.md

Instructions for AI coding agents working in this repository.
Read [docs/architecture.md](docs/architecture.md) and [docs/spec.md](docs/spec.md) before larger changes.

## Project

`git-when` is a Go command that shows when commits happen in Git repositories.
It writes terminal views, CSV, SVG, and a single-file HTML report.
The HTML report is built from an Astro frontend and embedded into the Go binary.

## Layout

```text
cmd/git-when/          the command (main.go, cli.go, output.go, report.go)
internal/discover/     repository discovery
internal/collect/      git log reading, author filters
internal/cache/        git log cache
internal/model/        aggregate model and JSON schema 1
internal/axis/         grids from two axes
internal/stats/        weekend index, summaries, UTC offsets
internal/render/       terminal, CSV, SVG, HTML (template.html is generated)
internal/progress/     progress line on stderr
internal/gittest/      test helpers that create repositories
frontend/              Astro source of the HTML report
docs/                  architecture.md, spec.md, examples.md, images/
```

## Commands

```sh
go build -o git-when ./cmd/git-when
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
golangci-lint run ./...

pnpm install --frozen-lockfile
pnpm dev:template        # Astro dev server with sample data
pnpm test:template       # Vitest
pnpm lint:template       # ESLint, declaration order only
pnpm build:template      # tsc --noEmit, then writes internal/render/template.html
pnpm check:template      # fails if template.html is stale
pnpm format              # Prettier, writes files
pnpm format:check        # Prettier, fails if a file is not formatted
```

Run every command from the repository root.

## Rules

### Compatibility

- Do not change user-visible behavior unless the task asks for it.
  This covers flags, defaults, validation, exit codes, output file names, and output content.
- Keep JSON schema 1, the flat `buckets` and `offsets` arrays, and the HTML data markers.
- Output must stay deterministic. Do not add timestamps or unsorted map iteration.
- When behavior changes on purpose, update `docs/spec.md` and, if affected, `docs/examples.md`.

### HTML report

- Never edit `internal/render/template.html` by hand.
  Change `frontend/` and run `pnpm build:template`, then commit the generated file.
- The report must work offline from `file://`.
  It has one data script between the markers and one application script. Nothing loads from the network.
  The only URL is the link to the GitHub repository.
  `build-template.mjs` checks this.
- `frontend/src/scripts/report/` must not touch the DOM, so Vitest can import it.
  DOM code goes in `frontend/src/scripts/ui/`.
- Component CSS uses `<style is:global>` because chart elements are created at run time.
- Add every UI text to both `en` and `ja` in `report/i18n.ts`.

### Code style

- Comments and docs are in short, plain English for non-native readers.
  Write one idea per sentence. Do not join sentences with semicolons, colons, or dashes.
- Comments explain constraints and reasons. Do not restate the code.
- Every exported Go identifier has a GoDoc comment that starts with its name.
- Japanese is allowed only in the `ja` translations, the Japanese language button label,
  and Unicode test data.
- Keep related code together. Do not split code into many tiny files.
  Aim for roughly 150-400 lines per file.
- Go code follows `.golangci.yaml` (gofumpt, goimports, golines at 100 columns).
- TypeScript runs in strict mode. Do not use `any`.
- Prettier formats everything except Go and generated files (`.prettierrc.json`, `.prettierignore`).

### Declaration order

Moving a declaration must not change its text.

Go files follow the Uber Go Style Guide.

1. `const`, then `var`, for values that do not belong to one type.
2. One block per type in this order.
   The type, its `iota` constants and its own `var` values come first.
   Functions that return only `T` or `*T` come next.
   Exported methods follow, and unexported methods come last.
3. Exported functions.
4. Unexported helpers, roughly in call order.

Go test files put package values and fake types first.
`TestMain` comes next, then tests in the order of the code they test.
Helpers go at the end.

TypeScript files use this order.

1. Imports.
2. Types and interfaces, exported ones first.
3. Module constants, exported ones first.
   A constant that reads another constant at load time comes after it.
4. Exported functions, including exported arrow function constants.
5. Unexported helpers.
6. Top-level statements such as setup calls.

`funcorder` in `.golangci.yaml` and `pnpm lint:template` check part of these rules.
typescript-eslint does not support TypeScript 7 yet.
So `package.json` installs `typescript` as `@typescript/typescript6` and TypeScript 7 as `@typescript/native`.
The `tsc` command is TypeScript 7.

### Tests

- Add or update tests with every behavior change.
- `gittest` repositories ignore the local Git config. Use them instead of real repositories.
- Tests must not write to the user cache directory. `cmd/git-when` tests already redirect it.

### Docs and images

- `docs/images/git-when-html.png` is a 1440x900 light theme screenshot of the plain template.
  Retake it when the report layout changes.
- `docs/images/examples/term-*.png` are colored terminal output rendered to PNG.
  Retake them when terminal output changes.

## Before you finish

1. Run the Go checks: `gofmt -l .`, `go vet ./...`, `go test ./...`, `golangci-lint run ./...`.
2. If `frontend/` changed, run `pnpm test:template`, `pnpm lint:template`, `pnpm build:template`,
   and `pnpm check:template`.
3. Run `pnpm format:check` if you changed any file that Prettier formats.
4. Update `docs/` when behavior, layout, or commands change.
5. Do not commit unless you are asked to.
