# AGENTS.md

## Project

Mod bisect tool with a Gio GUI (`cmd/mod-bisect-gui`) and a tview TUI
(`cmd/mod-bisect-tui`), sharing a common app layer (`pkg/app`), core search
logic (`pkg/core`), and UI contracts (`pkg/ui`).

## Useful commands

- `go doc <symbol>` — read package/type/function docs instead of opening
  dependency sources directly (e.g. `go doc gioui.org/x/component.ContextArea`,
  `go doc gioui.org/op/clip.Path`).
- `go build ./...` — build everything.
- `go vet ./...` — static checks.
- `go test -count=1 ./...` — run the full test suite (fresh, no cache).
- `gofmt -l <dir>` — list unformatted files.

## Conventions

- Prefer `go doc` for third-party APIs; don't read vendored/module sources
  unless necessary.
- Keep the layered architecture clean: `core` (pure logic) → `app` (controller + viewmodel) → `ui` (interfaces) → `gui`/`tui` (frontends). Shared UI-agnostic
  helpers live outside the frontends (e.g. `pkg/probe`).
- The manifest loader a mod targets is `mods.ManifestLoader`; the loader the
  user runs is `mods.RunLoader` (Fabric, NeoForge, Connector, Kilt).
- Probe results return an empty `PrimaryLoader` when nothing is detected; UI
  must not override the user's selection with an empty recommendation.
- On Windows the older files use CRLF line endings; `gofmt -l` reports them.
  New files use LF. Don't reformat the whole repo.
- All changes: `go build`, `go vet`, `go test -count=1 ./...`, and `gofmt` on
  touched files before finishing.
- Amend the Changelog.md before every commit or after every major feature is complete. Document only what actually affects the behavior of the app. Keep it brief.