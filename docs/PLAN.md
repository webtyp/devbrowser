---
PLAN: "feat: WithProfile — one persistent Chrome profile per project, so OPFS (the agent's models) survives between dev sessions"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `devbrowser`: a persistent profile per project

**Read [AGENTS.md](../AGENTS.md) first** (host-only repo, vendored chromedp, how to test without a
browser). Master plan:
[PWA_ARTIFACTS_MASTER_PLAN.md](https://github.com/webtyp/app/blob/main/docs/PWA_ARTIFACTS_MASTER_PLAN.md).

## Why

An application with an in-browser AI model copies its model files (≈ 1.2 GB) from the dev server
into the browser's private storage (OPFS) the first time, which takes about a minute; after that it
starts from OPFS in seconds. Today every dev session opens Chrome with a **throwaway** profile
(chromedp creates `chromedp-runner*` in the temp dir, and `ProfileCleaner` sweeps them), so OPFS —
and IndexedDB, cookies, the login session — is lost and the copy happens again every session. The
owner decided (2026-10-05): the dev browser keeps one profile per project, so the copy happens once
per model version, through the same code path as production.

## Design gate

1. **Prior art.** Playwright `launchPersistentContext(userDataDir)`, Puppeteer `userDataDir`,
   VS Code's / Vite's browser launchers: a fixed profile directory per workspace.
2. **Novice-name test.** `devbrowser.WithProfile(projectRoot)`: "use this project's profile".
3. **Complexity ledger.** +1 option, +1 exported function (`ProfileDir`), +1 constant.
4. **Where it belongs.** Here, next to `WithCache`.
5. **What it deletes.** Nothing; without the option the throwaway profile stays (tests, other tools).

## Stage 1 — `profile_dir.go` (new)

```go
// ProfilesDirName is where persistent profiles live, under the user's cache directory.
const ProfilesDirName = "webtyp/devbrowser/profiles"

// ProfileDir returns the persistent profile directory of the project at root:
// <os.UserCacheDir()>/webtyp/devbrowser/profiles/<first 12 hex chars of SHA-256 of the absolute root>.
// It does not create it.
func ProfileDir(root string) (string, error)

// WithProfile makes the browser use ProfileDir(root) as its Chrome profile (created with 0700 when
// missing), so storage (OPFS, IndexedDB, cookies) survives between sessions. When the directory
// cannot be resolved or created, or another live Chrome holds it (its SingletonLock points to a
// running pid), the browser falls back to a throwaway profile and logs why.
func WithProfile(root string) Option
```

`DevBrowser` gets the field `ProfileDir string // "" = throwaway profile`. `WithProfile` resolves
and creates the directory and sets the field; on error it leaves the field empty and stores the
reason in an unexported field logged once by `CreateBrowserContext` through `b.Logger`
(`lang.Translate("browser", "profile", "unavailable,", "using", "a", "temporary", "one:", reason)`).

## Stage 2 — `context.go`

In `buildAllocatorOptions`, when `h.ProfileDir != ""` and `!profileLocked(h.ProfileDir)`, append
`chromedp.UserDataDir(h.ProfileDir)`. `profileLocked(dir) bool`: read the `SingletonLock` symlink
(`os.Readlink`, target `"<hostname>-<pid>"`, the same parsing `profiles.go` already does — reuse
it, do not copy it) and return `processAlive(pid)`. When locked, log as above and do not set the
flag. Nothing else changes (cache flags, SPKI flags stay).

`ProfileCleaner` must never touch these directories: it only sweeps `ProfilePrefix` entries under
`os.TempDir()`; add a test that proves it with a `ProfileDir` under a temp root.

## Stage 3 — tests (root, `package devbrowser`, the style of `context_test.go`)

| Test | Proves |
|---|---|
| `TestProfileDir_StablePerProject` | same root (relative and absolute) → same dir; two roots → different dirs; the dir is under `os.UserCacheDir()` + `ProfilesDirName` (set `XDG_CACHE_HOME` with `t.Setenv` to a temp dir) |
| `TestWithProfile_SetsUserDataDir` | `b := &DevBrowser{}; WithProfile(tmpRoot)(b)` (as `context_test.go` builds one, without launching) → `buildAllocatorOptions` flags contain `user-data-dir` equal to `ProfileDir(tmpRoot)`; the directory exists with mode 0700 |
| `TestWithProfile_LockedFallsBack` | a `SingletonLock` symlink to `"<hostname>-<os.Getpid()>"` inside the profile dir → no `user-data-dir` flag |
| `TestBuildAllocatorOptions_NoProfileByDefault` | a DevBrowser without the option → no `user-data-dir` flag |
| `TestProfileCleaner_IgnoresPersistentProfiles` | `CleanStale` on a root holding a persistent profile dir (no `chromedp-runner` prefix) leaves it |

## Stage 4 — docs

`README.md`: a section "Persistent profile" (why: the models in OPFS, the session; how:
`devbrowser.New(ui, store, exit, devbrowser.WithProfile(projectRoot))`; where it lives; how to reset:
delete the directory).

## Acceptance

- `gotest` green. Never run `gopush` or `codejob`.
- `grep -rn '"user-data-dir"\|UserDataDir' --include=*.go . | grep -v "chromedp/\|_test"` → only `context.go`.

| Stage | Files | Done when |
|---|---|---|
| 1 | `profile_dir.go`, `devbrowser.go` (field) | option + dir |
| 2 | `context.go` | flag, lock fallback |
| 3 | tests | table green |
| 4 | `README.md` | documented |
