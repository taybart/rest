# Changelog

Release notes from the [GitHub releases](https://github.com/taybart/rest/releases), newest first.

## [v0.13.0](https://github.com/taybart/rest/releases/tag/v0.13.0) - 2026-11-26

### Behaviour changes

- **`fail()` stops the script.** In `after` hooks and server scripts, the code
  after `fail(msg)` used to keep running (hence `return fail(...)`). Now it
  stops right there. `return fail(...)` still works. `fail` is a lua error now,
  so `pcall` / `catch` can catch it.
- **`cond ? a : b` only evaluates the branch it picks.** HCL used to evaluate
  both (any errors from the other one were dropped). The two branches are no
  longer converted to a common type: `x ? 1 : "a"` is the number `1`, not
  `"1"`.
- **`env("KEY", default)` only evaluates the default when `KEY` is unset or
  empty.**
- **`-h` / `--help` on a file with a `cli` block print its usage and flags,
  and `fn` doesn't run.** Before, with `no_flags = true` and no flags, `--help`
  was swallowed and `fn` ran with no arguments.
- **Server flags with `-f` are an error.** `rest -s -f site.rest --addr ...`
  used to ignore the flag. The file's `server` block has a setting for each.
- **`/__quit__` is `POST` only, and only on a bare mock server** (ex. `rest -s`).
  A server with handlers, a `directory`, or a `proxy` doesn't have it.
- **The old `_` function names warn:** `json_enc`/`json_dec`, `b64_enc`/`b64_dec`,
  `jwt_sign`/`jwt_enc`/`jwt_dec` are now `json::enc`, `b64::dec`, `jwt::sign`,
  `jwt::dec`, ... (`try_exports` is `exports::try`). They still work.
- **The `pages` Go package moved to midnight.** Go apps that serve pages import
  it from midnight now. The `.rest` side is unchanged.

### Client

- **`test` blocks:** run requests by label in order, with `cleanup` steps that
  run even when a step fails. `rest -f FILE -T` runs every test, `-T NAME` one.
  Each test gets fresh exports and cookies. `--list` lists them too.
- **`op::read("op://vault/item/field" [, account])`** reads a secret from
  1Password with the `op` CLI. It's cached in memory for the run, never on disk.
  `env("PASSWORD", op::read(...))` lets an env var override it without calling
  `op`.
- **`lua::enc(value)`**: an HCL value as a lua / midnight literal.
- **`tmpl()` takes any values** (numbers, lists, objects: `{{.page.title}}`),
  not just strings, and `{{lua .x}}` writes a lua literal.
- **`b64::enc` / `b64::dec`.**

### CLI blocks

- **`usage = "..."`** on the `cli` block: printed by `-h`/`--help` (above the
  flags) and by `fail.usage()`, and available as `cli.usage`.
- **`fail(msg [, code])`** in `fn` stops it with a clean error (no line of code)
  and exits with `code` (default 1). **`fail.usage([msg])`** prints the message,
  then the usage, and exits 2.

### Server

- **Dev mode (`--dev`):**
  - The page reloads (morphed in, keeping scroll, focus and input) when rest
    restarts, and swaps a stylesheet when its `.css` changes.
  - Script errors show in the browser.
  - Handler caches are off.
  - Works with `run -w` without a restart per change.
- **Precompressed files:** `app.js.br` / `app.js.gz` next to `app.js` in
  `directory` are sent in its place when the client accepts them. A stale one
  (older than the original) is ignored.
- **`dynamic` blocks** for `handler` and `middleware` (HCL's dynblock, like
  Terraform's), ex. a handler per page from a list in `locals`.
- **`browser_cache`** on handlers sets the `Cache-Control` max-age separately
  from `cache` (`"0s"` sends no-cache).
- **`name`** on the `server` block: the title of `-e openapi`, the install
  comment of `-e systemd`, and a prefix on the server's log lines.
- **`-f` can be a folder:** `rest -s -f /var/www/site.com` serves the `.rest`
  file directly in it that has a `server` block.
- **`rest -e systemd`** writes a `rest@.service` template unit, one process per
  site, run from `/var/www/<site>`.

### Fixes

- Errors that start or end with `[` / `]` print intact (`taybart/log` v1.6.9).
- Page morphing in dev mode.
- Nix modules and dependency hashes.

## [v0.12.0](https://github.com/taybart/rest/releases/tag/v0.12.0) - 2026-11-13

### Features

 - **Pages**: you can now render pages (#321492c).
   - Pages moved into their own `rest/pages` package so APIs can use it, with `embed.FS`
 support, `pages.LuaFuncs` and docs in `doc/PAGES.md` (#561bdb9).
   - New `pages.JSON` helper for passing Go data to page scripts (#ec9cef5).
   - Pages respect `omitempty`/`omitzero`, so Lua defaults fill in empty fields (#a1753d7).
   - Added a client-side example (#8312f33).
 - **Config**: new `config.default_language` (`"lua"` | `"midnight"`) sets the default parser
 (#9d0a95d).
 - **Midnight**: renamed the templating engine to midnight, moved it from `/x/` to main, and
 updated to the new syntax with `define:vars` support (#3728bcf).

### Performance

 - Pages render faster (#786d182).
 - Pages are now gzipped, using `gzip.BestSpeed` (#e67418e, #dba56b3).
 - `json.encode` key ordering and an `os.Stat` cache (#dba56b3).

### Fixes

 - Fixed a lock copy and the submit event in pages (inside the pages squash, #561bdb9).

### Dependencies

 - Bumped `github.com/taybart/midnight` to `v0.0.0-20261109172334-f697c48f2746` (#53f4705,
#bc88495, #5e75ab8, #b4f0f5b, #fd82dbb).

## [v0.11.0](https://github.com/taybart/rest/releases/tag/v0.11.0) - 2026-11-05

### Breaking changes

  - The multi-request "client" templates for go and js are removed. Exports now produce one snippet per request, and the built-in templates are curl, go and js.
  - Defining the same local twice in one file is now an error.
  - Default User-Agent is now rest-client/1.0 -> rest-client/2.0.

### Features

  - OpenAPI import and export (#13):
    - rest -i spec.json scaffolds a .rest file from an OpenAPI 3.x spec. --side client|server|both picks what it generates (default both). Input must be JSON.
    - The OpenAPI export turns server handlers into an OpenAPI 3.1 spec. expect blocks become parameters and request bodies, and named schemas become components.schemas.
  - `schemas` block: define a request or response shape once and use it as schemas.<name> in expect blocks. Postman and OpenAPI exports keep the names.
  - Handlers can validate requests: an expect block on a handler checks the incoming request's path, query, headers and body, and rejects mismatches with a 400.
  - `proxy` blocks:
    - proxy = "host" (or a proxy { target, strip, headers, insecure } block) on a handler, or on the server as a fallback for unmatched routes.
    - Streams SSE and chunked responses, forwards websockets, and sets X-Forwarded-* headers.
  - Erde scripting: any script can be written in erde with an <<ERDE heredoc. The experimental match statement is enabled.
  - WASM playground: a first version of a browser playground for rest.
  - Custom export templates:
    - Pass your own foo.<lang>.tmpl file.
    - Templates get the request split into parts (BaseURL, Query, RequestHeaders, Auth, UserAgent, …) alongside combined .URL() and .Headers() methods.
    - lit writes a correctly escaped literal for the template's language (sh, go, js, lua), and ident makes a valid identifier.
  - env("KEY", "default"): env takes an optional default value.
  - New lua helpers: html.escape/html.unescape and url.encode/url.decode.
  - Postman export runs expect checks as tests: status, header and JSON-schema body tests, with named schemas defined once at the collection level.

### Fixes

  - Locals are evaluated in dependency order, so references work in any order. Before, this failed intermittently.
  - Locals across imports: the importing file overrides its imports, later imports override earlier ones, and a library can still reference a local that only the importing file defines.
  - Namespaced imports no longer prefix request labels more than once (lib::lib::one).
  - Query parameters: a query string written inline in a request URL is no longer dropped when the request also has a query block. The block wins on conflicts.
  - Exports respect skip.
  - Postman export: URL path segments, query parameters, auth and JSON bodies are now exported correctly. Checked by running the collection through newman.

### Build

  - Nix builds use gomod2nix, so the vendor hash no longer needs manual updates (#29).
  - go-erde is pulled as a normal dependency instead of through a replace directive.

## [v0.10.2](https://github.com/taybart/rest/releases/tag/v0.10.2) - 2026-10-28

### Features

- **`--flags` CLI override** (`3e81b95`): Added `--flags` to force built-in `rest` CLI flags (such as `-l`, `-b`, and `-f`) and bypass `cli` block execution on files configured with `config { no_flags = true }` (e.g., for editor integrations like `rest.nvim`).
- **`lib.dump` & `tools.dump()`** (`dede613`): Renamed `lib.print_req_res` to `lib.dump` and added `tools.dump()` in Lua to print both the HTTP request and response dumps.

### Bug Fixes

- **Include request body in `rest.req.dump`** (`dede613`): Restore the consumed request body reader before dumping the HTTP request so `rest.req.dump` (and `lib.dump` / `lib.print_req`) includes the sent request body.

## [v0.10.1](https://github.com/taybart/rest/releases/tag/v0.10.1) - 2026-10-27

### Features

- **Default `Content-Type` header for JSON bodies** (`a7dc3aa`): Automatically adds `Content-Type: application/json` when a request has a valid JSON body and no `Content-Type` header is specified. Can be disabled per file via `config { default_content_type = false }`.
- **ULID & NanoID support** (`5165986`):
  - **Lua `ulid` module**: `ulid.new([ms])` / `ulid([ms])`, `ulid.is_ulid(str)`, and `ulid.timestamp(str)`.
  - **Lua `nanoid` module**: `nanoid.new([size [, alphabet]])` / `nanoid([size [, alphabet]])` and `nanoid.is_nanoid(str [, size])`.
  - **Lua `uuid` module**: Added `uuid.is_uuid(str)` alongside `uuid.newv4()` and `uuid.is_uuidv4(str)`.
  - **HCL & `expect`**: Added `ulid()` HCL function and `is.ulid` expectation matcher.

### Performance & Binary Size

- **Native Go Lua modules & dependency cleanup** (`9e7e995`, `61d6389`):
  - Replaced embedded Lua `json` and `uuid` implementations with native Go functions.
  - Replaced `golang.design/x/clipboard`, `gluamapper`/`mapstructure`, `golang.org/x/text/cases`, and `go/format` with lightweight stdlib implementations, reducing binary size by ~1.3 MB.

## [v0.10.0](https://github.com/taybart/rest/releases/tag/v0.10.0) - 2026-10-26

### New stuff

- `expect` body & header type matchers (`is.uuid`, `is.jwt`, `is.int`, `is.string`, `is("regex:...")`, etc.)
- Pre-canned `after` hooks (`lib.expected`, `lib.print_req_res`, `lib.inspect_res`, `lib.inspect_jwt`) and `lib::` helper functions (`lib::inspect_res`, `lib::export_res`, `lib::copy_res`, `lib::inspect_jwt`)
- `after = [...]` now accepts a list of hooks
- JWT signing, decoding, and verification in HCL (`jwt_sign`, `jwt_dec`) and Lua (`jwt.sign`, `jwt.decode`, `jwt.verify`, `jwt.is_valid`)
- Go-backed `base64` Lua module with URL-safe and unpadded options (`base64.encode(str, { url = true, raw = true })`)
- Server `middleware` blocks with shared `rest.ctx`
- Server `kv = { ... }` seeding, `setup` script, and concurrency-safe `kv` (`kv.del`, `kv.all`, nested tables)
- Cookie helpers (`rest.req:cookie`, `rest.res:cookie`, and structured `cookies` table in server responses)
- CLI `rest.loop()` support from `fn` with automatic `input` reading
- Bundled `rest` agent skill (`skills/rest/SKILL.md`)

**Full Changelog**: https://github.com/taybart/rest/compare/v0.9.3...v0.10.0

## [v0.9.3](https://github.com/taybart/rest/releases/tag/v0.9.3) - 2026-10-24

### Features

- Marshal body as form if header is present

**Full Changelog**: https://github.com/taybart/rest/compare/v0.9.2...v0.9.3

## [v0.9.2](https://github.com/taybart/rest/releases/tag/v0.9.2) - 2026-10-23

### Features

- add `rest.req:query("key")` and `rest.req:form("key")`

**Full Changelog**: https://github.com/taybart/rest/compare/v0.9.1...v0.9.2

## [v0.9.1](https://github.com/taybart/rest/releases/tag/v0.9.1) - 2026-10-21

### Features

- `cli.rest` for using the "rest" of the arguments

### Fixes

- Don't nuke exports table between request runs and cli

## [v0.9.0](https://github.com/taybart/rest/releases/tag/v0.9.0) - 2026-10-21

### New stuff

- server `--tailscale` flag, uses the tailscale api to add a tls cert to listen on you tailnet
- dynamic types in `try_exports`
- ability to use a heredoc style stdin with the rest cli (`rest <<EOF request "test" {...EOF`)
- use `github.com/taybart/args` for cli block flag parsing, this adds default values and short versions of the flag name
- `config { no_flags = true }` to turn off reserved flags for use in cli scripts

## [v0.8.4](https://github.com/taybart/rest/releases/tag/v0.8.4) - 2026-09-22

Fix large file downloads in rest server

## [v0.8.3](https://github.com/taybart/rest/releases/tag/v0.8.3) - 2026-09-09

Add `raw("connect")` for marking socket messages that are raw

## [v0.8.2](https://github.com/taybart/rest/releases/tag/v0.8.2) - 2026-09-09

Update deps

## [v0.8.1](https://github.com/taybart/rest/releases/tag/v0.8.1) - 2026-05-07

fix exports table sync between requests and in cli overwriting values

## [v0.8.0](https://github.com/taybart/rest/releases/tag/v0.8.0) - 2026-05-07

feat: cli blocks [docs](https://github.com/taybart/rest/blob/v0.8.0/doc/CLIENT.md#cli-block)

## [v0.7.3](https://github.com/taybart/rest/releases/tag/v0.7.3) - 2025-12-13

fix: nil checks
fix: respect server.Config.Quiet on "listening on.." message

## [v0.7.2](https://github.com/taybart/rest/releases/tag/v0.7.2) - 2025-12-11

Add `--list` flag
Add `--spa` and `spa = false` to set 404 behavior in servers

## [v0.7.1](https://github.com/taybart/rest/releases/tag/v0.7.1) - 2025-12-09

Fix issue where method in "from" block was not copied when receiving block did not have one defined

## [v0.7.0](https://github.com/taybart/rest/releases/tag/v0.7.0) - 2025-11-08

Rename post_hook -> after

## [v0.6.1](https://github.com/taybart/rest/releases/tag/v0.6.1) - 2025-10-31

Fixes to sockets

## [v0.6.0](https://github.com/taybart/rest/releases/tag/v0.6.0) - 2025-10-31

Working `exports`! fixes: #24

## [v0.5.2](https://github.com/taybart/rest/releases/tag/v0.5.2) - 2025-10-14

Update docs...

### Features

- `server.Serve`

## [v0.5.1](https://github.com/taybart/rest/releases/tag/v0.5.1) - 2025-10-14

### Bugfixes

- Decoding locals blocks now can use other locals blocks above them, note: cannot use locals in the same block as it leads to race conditions
- Allow leading `.` in basic template variables
- fixed how server cors works so that it allows all when turned on
- check all header values when expecting instead of first value in key

### Features

- Namespaced imports `filename::request_label`, can be turned off with `config { namespace_imports = false }`
- Ability to skip all imported blocks `config { skip_imported = true }`
- Add `expect` blocks for more complicated testing
- uuid/nanoid functions in templates and hcl
- Add `tools.lua` into lua runtime

updated server block changes are in `docs/SERVER.md` along with expect blocks in `docs/CLIENT.md`

## [v0.5.0](https://github.com/taybart/rest/releases/tag/v0.5.0) - 2025-10-03

Add server blocks and custom responses

```hcl
server {
  address = "localhost:18080"
  tls = "test/keys/example.com"
  quiet = true
  origins = ["*"]
  headers = {"x-custom-header": env("HEADER_VALUE")}
# serve a directory
  directory = "./test" # directory will override response
# not required, override the default response if needed
  response {
    status = 200
    body = {
      "custom": "response"
    }
  }
}
```

## [v0.4.2](https://github.com/taybart/rest/releases/tag/v0.4.2) - 2025-09-23

Add lua colors

## [v0.4.1](https://github.com/taybart/rest/releases/tag/v0.4.1) - 2025-07-20

feat: add labels and blocks for single request export

## [v0.4.0](https://github.com/taybart/rest/releases/tag/v0.4.0) - 2025-07-02

**Full Changelog**: https://github.com/taybart/rest/compare/v0.3.0...v0.4.0

### Added

- socket blocks
- socket repl
- templates and "basic" templates
- copy_from
- form function for url encoded bodies
- imports

## [v0.3.0](https://github.com/taybart/rest/releases/tag/v0.3.0) - 2025-06-17

### Client

Add lua interpreter for post processing, better body expression

### Server

__echo__ endpoint

## [v0.2.3](https://github.com/taybart/rest/releases/tag/v0.2.3) - 2025-04-10

Add server flags for setting allowed origins and tls key/cert

## [v0.2.2](https://github.com/taybart/rest/releases/tag/v0.2.2) - 2023-04-12

_No release notes._

## [v0.2.1-rc](https://github.com/taybart/rest/releases/tag/v0.2.1-rc) - 2022-09-06 (pre-release)

_No release notes._

## [v0.2.0-rc](https://github.com/taybart/rest/releases/tag/v0.2.0-rc) - 2022-09-05 (pre-release)

Move to hcl instead of bespoke language

## [v0.1.1](https://github.com/taybart/rest/releases/tag/v0.1.1) - 2020-12-08

Fix #15

## [v0.1.0](https://github.com/taybart/rest/releases/tag/v0.1.0) - 2020-11-02

Add support for runtime variables
```http
http://localhost:8080
POST /login
{
  "username": "test",
  "password": "password"
}

take auth_token as AUTH_TOKEN

---

http://localhost:8080
Authorization: Bearer ${AUTH_TOKEN}
GET /account
```

## [v0.0.1](https://github.com/taybart/rest/releases/tag/v0.0.1) - 2020-07-28

_No release notes._
