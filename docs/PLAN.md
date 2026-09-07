---
PLAN: "feat: trust the dev TLS certificate by SPKI pin, no CA installation"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> **UNBLOCKED (2026-09-07).** `webtyp.com/server/httpd` now exports
> `DevCertSPKI()` (`server/httpd/spki.go:15`). This plan was dispatched but never
> completed, and it is the direct cause of a live bug — see below.

## Prerequisite — install the test runner

External agents run in isolated environments where `gotest` is not installed.
Run this **before anything else**; the acceptance criteria depend on it:

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

Then use `gotest` for the whole suite and `gotest -run TestName` for one test.
Never call `go test` directly: `gotest` handles `-vet`, `-race`, `-cover`, the
WASM suite and the README badges.

# Plan — trust the development certificate without touching the OS

## The live bug this fixes

`webtyp dev` in a project cannot open its own browser:

```
13:10:03  SERVER   Starting Internal Server on port: 8080
13:10:03  BROWSER  Error opening DevBrowser: error navigating to
                   https://localhost:8080/: page load error
                   net::ERR_CERT_AUTHORITY_INVALID
```

Reproduced against the running server:

```
$ curl    https://localhost:8080/     → exit 60 (unable to get local issuer certificate)
$ curl -k https://localhost:8080/     → 200
$ openssl … -issuer -subject          → issuer=O=WebTyp Dev CA, subject=O=WebTyp Dev CA
```

The server is correct; the certificate is a self-signed leaf, as designed. What
is missing is this package telling Chrome to trust that one public key — the
work this plan describes and which was never implemented.

## Context (the executing agent has none — read this fully)

`webtyp.com/devbrowser` launches and drives Chrome through `chromedp`. The
allocator flags are built in `context.go:12-50`:

```go
opts := append(chromedp.DefaultExecAllocatorOptions[:],
    chromedp.Flag("headless", h.Headless),
    chromedp.Flag("enable-automation", false),
    chromedp.Flag("use-fake-ui-for-media-stream", true),
    ...
)
allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
```

The development server is moving to HTTPS by default (`httpd` `DevTLS`), so the
browser must trust a certificate no public authority signed.

### Why not install a local CA

The obvious approach — a `mkcert`-style CA added to the OS trust store — costs:
elevated privileges on Linux (`/usr/local/share/ca-certificates` plus
`update-ca-certificates`) and macOS (`security add-trusted-cert`); a second,
separate installation into the NSS database for Firefox and Chrome on Linux
(`certutil`); and a machine left permanently trusting a private CA after the
tool is uninstalled.

None of that is necessary here, because **this tool launches the browser it
needs to convince**. Chrome accepts `--ignore-certificate-errors-spki-list`,
which trusts exactly the listed public keys and nothing else. It is scoped to
the browser instances this package starts, requires no privileges, behaves
identically on Linux, macOS and Windows, and leaves nothing behind.

### What this does not solve — say so, do not paper over it

A phone on the LAN opening `https://192.168.x.x` is **not** this browser and
cannot be given a flag. That case still needs a certificate the device trusts.
It is out of scope here and must not be worked around by disabling TLS
verification globally.

A related correction worth recording, because it changes what to test:
`http://localhost` is already a *secure context* in Chrome and Firefox, so
`getUserMedia` (camera and microphone) works there **without** TLS. What
genuinely breaks without HTTPS is everything on a non-localhost origin — a phone
on the LAN — plus `Secure` cookies, `SameSite=None`, HSTS and mixed content.
Those are the reasons to serve HTTPS in development; camera access on localhost
is not one of them.

## Design gate

**1. Prior art.** Playwright and Puppeteer expose `ignoreHTTPSErrors`, which
disables verification wholesale for the session. Cypress does the same. Chrome
DevTools' own testing guidance recommends the SPKI list precisely because it is
the narrow version: verification stays on for every other origin. Vite's
`@vitejs/plugin-basic-ssl` and `mkcert` take the CA-installation route and pay
the privilege cost this plan avoids.

Choosing the SPKI pin over `ignoreHTTPSErrors` is deliberate: the blanket flag
would also trust a genuinely broken certificate on any site the dev browser
visits, which is a debugging trap and a security regression in a tool that
browses arbitrary pages.

**2. Novice-name test.** `Config.TrustDevCertSPKI string` states what it is: the
SPKI hash of a development certificate to trust. Rejected: `IgnoreTLSErrors`
(describes a weaker, different behaviour and invites the blanket flag),
`InsecureSkipVerify` (borrowed from `crypto/tls` where it means something
broader).

**3. Ledger.**

```
Concepts to learn                +1   (one config field)
Privileged operations            +0   (vs +1 sudo prompt for the CA route)
OS-specific code paths           +0   (vs +3 for system store and NSS)
Origins whose TLS is unverified  +0   (the pin trusts one key, not every site)
Machine state left behind         0   (vs a permanently trusted private CA)
```

**4. Where it belongs.** Launching Chrome with the right flags is this package's
single responsibility. No new package.

**5. What it deletes.** Nothing yet — this is new capability. It prevents a CA
installer from ever being written.

## Stage 1 — configuration

Add to the browser configuration struct in `config.go`:

```go
// TrustDevCertSPKI is the base64-encoded SHA-256 of a development certificate's
// SubjectPublicKeyInfo. When set, the launched browser trusts exactly that
// public key; verification stays on for every other origin.
//
// Obtain it from webtyp.com/server/httpd.DevCertSPKI(). Empty = no pin.
TrustDevCertSPKI string
```

Never derive it inside this package: `httpd` owns the certificate, so `httpd`
owns its hash. Recomputing it here would fork that responsibility.

## Stage 2 — the flag

In `context.go`, after the existing flags and before
`chromedp.NewExecAllocator`:

```go
if h.TrustDevCertSPKI != "" {
    opts = append(opts, chromedp.Flag(FlagSPKIList, h.TrustDevCertSPKI))
}
```

with

```go
// FlagSPKIList trusts the listed SubjectPublicKeyInfo hashes and nothing else.
// It is NOT --ignore-certificate-errors: verification stays on everywhere else.
const FlagSPKIList = "ignore-certificate-errors-spki-list"
```

Chrome accepts a comma-separated list. Accept one value; joining multiple is not
needed and would invite a list nobody audits.

**Do not** add `--ignore-certificate-errors`, `--allow-insecure-localhost`, or
`--disable-web-security`. If a future need seems to require one, that is a
finding to report, not a flag to add.

## Stage 3 — the consumer-shaped test

The ecosystem rule: *an API is not published until a consumer-shaped test,
inside the library itself, proves it.* The last attempt at this feature shipped
`DevCertSPKI()` in `httpd` with only an isolated test, so nothing noticed that
no consumer ever called it. Do not repeat that here.

Add a test in this package that goes through the real path a consumer uses:

```go
// The value a consumer actually passes comes from httpd.DevCertSPKI(). This
// test uses a fixed 44-character base64 string of the same shape, asserts it
// reaches the allocator verbatim, and asserts the flag name is the SPKI list
// and not a blanket bypass.
```

It must fail if the flag is dropped, renamed, or replaced by
`--ignore-certificate-errors`.

The wiring in `webtyp.com/app` — reading `httpd.DevCertSPKI()` and passing it to
`devbrowser.New` — is tracked in
https://github.com/webtyp/app/blob/main/docs/PLAN.md. It is a separate
repository, but this plan is worthless without it: **both must land** before
`webtyp dev` opens a browser again.

## Constraints

- **No hardcoded strings.** The flag name is the constant above.
- **Minimal surface.** One exported field and one exported constant.

## Tests

The allocator options are built in a function that currently also starts the
browser. Extract the option-building into an unexported pure function returning
`[]chromedp.ExecAllocatorOption` so it can be tested without launching Chrome —
a test that starts a browser is not acceptable here.

1. Empty `TrustDevCertSPKI` → no `ignore-certificate-errors-spki-list` option.
2. A set value → exactly one such option, carrying that value.
3. No option named `ignore-certificate-errors`, `allow-insecure-localhost` or
   `disable-web-security` is ever produced. This is a regression guard: those
   flags are how this feature would silently become a blanket bypass.

## Acceptance criteria

1. `grep -rn "ignore-certificate-errors\"\|allow-insecure-localhost\|disable-web-security" --include='*.go' .` → empty.
2. `go build ./... && go vet ./... && go test ./...` → clean.
3. Test 3 passes.

## Stages

| # | Stage | File(s) | Gate |
|---|---|---|---|
| 1 | config field | `config.go` | compiles |
| 2 | flag + extracted option builder | `context.go` | tests 1, 2, 3 |
| 3 | consumer-shaped test | `context_test.go` | fails when the flag is dropped |

Sequential.
