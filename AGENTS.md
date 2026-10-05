# AGENTS.md — webtyp/devbrowser

Working notes for AI agents operating in this repository. End-user docs: [README.md](README.md).

## What this repo is

The development browser of the webtyp daemon (`webtyp.com/app`): it launches Chrome through a
**vendored** `chromedp` (`./chromedp`, `./cdproto`), reloads it on changes and exposes browser tools
over MCP (`mcp-*.go`).

## This repo does NOT compile to WASM. The standard library is legitimate here.

It runs on the developer's machine only. `os`, `path/filepath`, `crypto/sha256`, `net/http` are
correct here — do **not** replace them with `webtyp.com/*` browser packages.

## The build that defines "done"

```bash
go install webtyp.com/devflow/cmd/gotest@latest   # once
gotest
```

Tests that would launch a real browser must not be required to pass in an environment without
Chrome: test option building and file logic with unit tests (see `context_test.go`, which reads the
allocator flags without launching anything).

## Rules

- Do not edit `chromedp/`, `cdproto/` (vendored).
- Every repeated string (flag names, directory names) is a named constant.
- Messages to the developer go through `b.Logger` with `webtyp.com/fmt/lang` like the rest of the code.
