# PROJECT KNOWLEDGE BASE

**Generated:** 2026-07-06T08:56:07Z
**Commit:** b18b1bc
**Branch:** main

## OVERVIEW
Go CLI untuk otomasi registrasi akun ChatGPT/K12: Gmail dot-trick, IMAP OTP, proxy, TLS fingerprinting, batch workers, token export.

Module `github.com/verssache/chatgpt-creator`; repo name `K-12-reverse`; `go.mod` pins `go 1.25.5`.

## STRUCTURE
```text
K-12-reverse/
├── cmd/                 # CLI entrypoints: register, generator, checkmail, testotp
├── internal/register/   # core signup/login/K12/batch orchestration
├── internal/email/      # Gmail IMAP, dot-trick pools, temp email fallback
├── internal/sentinel/   # OpenAI Sentinel challenge/token generation
├── internal/config/     # config.json schema + PROXY override
├── internal/chrome/     # browser/TLS profile mapping
├── internal/util/       # password, random names, proxy, trace helpers
├── internal/ui/         # terminal colors/banner/clear screen
├── internal/updater/    # GitHub release update check
├── data/                # generated dot lists, token JSON, session state
├── config.json          # local runtime config; may contain credentials
├── results.txt          # runtime output, ignored but present in repo
└── test_auth.go         # standalone manual auth script, not Go test
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Start main app | `cmd/register/main.go` | Menu, setup wizard, resume, export tokens |
| Batch registration | `internal/register/batch.go` | Workers, retry accounting, `data/session.json`, output writes |
| Signup flow | `internal/register/flow.go` | CSRF/signin/register/OTP/create/callback/K12 handoff |
| Login/zombie rescue | `internal/register/flow_login.go` | Existing/partial account recovery |
| K12 invite + token | `internal/register/k12.go` | Workspace switch, token extraction |
| HTTP/TLS client | `internal/register/client.go` | Proxy, TLS profile, shared request logging |
| Gmail OTP | `internal/email/gmail_imap.go` | Exact `To` match, unread filter, 5-minute window |
| Gmail dot pool | `internal/email/gmail_pool.go` | Consumes `data/list_*.txt` across accounts |
| Dot list generation | `internal/email/dot_generator.go` | `cmd/generator` uses this |
| Config schema | `internal/config/config.go` | `PROXY` env overrides `config.json` |

## CODE MAP
| Symbol | Type | Location | Refs | Role |
|--------|------|----------|------|------|
| `main` | func | `cmd/register/main.go:19` | manual | interactive app shell |
| `RunBatch` | func | `internal/register/batch.go:166` | main | concurrent registration engine |
| `registerOne` | func | `internal/register/batch.go:49` | `RunBatch` | per-account worker flow |
| `NewClient` | func | `internal/register/client.go:37` | register/login | TLS/proxy client setup |
| `RunRegister` | method | `internal/register/flow.go:286` | worker | signup + OTP + account creation |
| `RunLogin` | method | `internal/register/flow_login.go:17` | zombie rescue | login recovery path |
| `RunK12Flow` | method | `internal/register/k12.go:175` | register/login | K12 workspace invite/token |
| `GetVerificationCodeViaIMAP` | func | `internal/email/gmail_imap.go:25` | register/login | Gmail OTP polling |
| `NewMultiGmailPool` | func | `internal/email/gmail_pool.go:26` | main | multi-account dot-list pool |
| `GenerateDotTrick` | func | `internal/email/dot_generator.go:11` | wizard/generator | Gmail aliases |
| `BuildSentinelToken` | func | `internal/sentinel/challenge.go:68` | account create | Sentinel challenge wrapper |
| `Load` / `Save` | funcs | `internal/config/config.go:46` | main | JSON config IO |

## CONVENTIONS
- No build wrapper. Run entrypoints directly with `go run` or build with `go build`.
- No linter config. Use `gofmt` before changes.
- No `_test.go` suite. Existing checks are standalone CLIs with printed output.
- Keep `internal/` packages private; entry binaries stay under `cmd/` unless intentionally manual like `test_auth.go`.
- `config.json` and `data/accounts_*.json` may contain secrets/tokens. Do not print or paste values.
- `PROXY` env var overrides `config.json.proxy` only.

## ANTI-PATTERNS (THIS PROJECT)
- Do not normalize Gmail dot variants during IMAP OTP matching; exact `To` match prevents worker OTP theft.
- Do not add a global IMAP lock inside `GetVerificationCodeViaIMAP`; code comment says lock was removed to avoid deadlocks.
- Do not convert manual tools into network tests without explicit opt-in; they hit Gmail/OpenAI/K12 live services.
- Do not trust `results.txt`, `data/accounts_*.json`, `export_tokens.txt`, or `config.json` as safe sample data.
- Do not add npm/pnpm/yarn/Bun commands; this repo has no Node toolchain.

## UNIQUE STYLES
- Indonesian user-facing CLI copy mixed with English code names.
- Telegram-style status output: emoji + worker ID + timestamp + color via `internal/ui`.
- Gmail mode stores one queue/token file per base username: `data/list_<username>.txt`, `data/accounts_<username>.json`.
- Zombie account handling switches signup failures into login flow when possible.

## COMMANDS
```bash
go run cmd/register/main.go
go run cmd/generator/main.go <email> <output_file>
GMAIL_BASE=x GMAIL_APP_PASSWORD=y go run cmd/checkmail/main.go
go run cmd/testotp/main.go
go run test_auth.go
go build -o k12-creator ./cmd/register
gofmt -w <changed .go files>
```

## NOTES
- README was checked against discovery; Go version requirement was stale versus `go.mod`.
- `k12-creator.exe` is committed although binary outputs should stay ignored.
- `.gitignore` ignores `results.txt` and `blacklist.json`; it now also ignores `.slim/deepwork/` local agent state.
- `export_tokens.txt` and `data/session.json` are generated by runtime but not currently ignored.
