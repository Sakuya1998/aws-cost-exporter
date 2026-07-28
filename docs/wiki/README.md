# Wiki authoring guide

`docs/wiki` is the single source of truth for the public GitHub Wiki of the current stable release.

## Rules

- English pages use `Page-Name.md`; Simplified Chinese pages use `Page-Name-zh-CN.md`.
- Every content page begins with links to both language variants.
- `_Sidebar.md` links every published page exactly once per language.
- `_Footer.md` identifies v0.3.0 and warns that the published Wiki is generated.
- The synchronization workflow publishes every Markdown file in this directory except this authoring guide.
- Edit these files through a pull request. Direct GitHub Wiki edits are overwritten.
- Use only placeholder account IDs, ARNs, buckets, costs, and environment-variable names.
- The Wiki documents the current stable release. Historical contracts belong in release records.

Run `go test ./test/docs/... ./test/ci/...` before submitting changes.
