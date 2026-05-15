# CPA Compatibility Import Plan

## Goal

Use `sub2api` as the only online gateway and import legacy `MyCPA` data at startup so existing CPA data can continue to work without keeping `MyCPA` in the request path.

This implementation targets the first usable compatibility phase:

- Import legacy `auths/*.json` into `sub2api` accounts
- Import legacy `config/config.yaml` top-level `api-keys` into `sub2api` API keys
- Reuse original client keys unchanged
- Support GitStore bootstrap via:
  - `GITSTORE_GIT_URL`
  - `GITSTORE_GIT_USERNAME`
  - `GITSTORE_GIT_TOKEN`
- Keep `sub2api` full distribution logic as the only runtime distributor

## Core Decision

The first implementation does not keep legacy keys bound to a `group`.

Reason:

- In `MyCPA`, one client key can naturally dispatch across multiple providers.
- In `sub2api`, one API key bound to one group would force a single platform path.
- `sub2api` already supports scheduling ungrouped accounts by requested platform.

So the compatibility pool is:

- one synthetic legacy user
- legacy accounts imported as ungrouped accounts
- legacy keys imported as ungrouped API keys

This is the closest behavior to old CPA dispatch while preserving original keys.

## Startup Architecture

Boot order:

1. `AUTO_SETUP`
2. `initializeApplication`
3. `CPAImportBootstrap.Run(...)`
4. `ListenAndServe`

Import runs before the server starts listening, so the first request already sees imported accounts and keys.

## Compatibility Scope

### Included in phase 1

- Read legacy source from:
  - Git clone snapshot
  - local source directory
- Parse `auths/*.json`
- Parse `config/config.yaml` top-level `api-keys`
- Map common legacy providers to `sub2api`
- Create or update imported accounts idempotently
- Create original keys with `CustomKey`
- Persist import mapping state in database
- Re-import on each startup without duplicating data
- Best-effort import per-auth proxy URLs into `sub2api` proxy records

### Not included in phase 1

- Usage history migration
- Subscription history migration
- One-key-one-user legacy split
- Bidirectional sync back to CPA
- Live watcher or hot sync
- Full migration of every niche legacy provider extension

## Source Model

### Git source

When `GITSTORE_GIT_URL` is configured, startup will:

1. clone the GitStore repo to a temp directory
2. read:
   - `auths/*.json`
   - `config/config.yaml`
3. delete the temp snapshot after import

Optional:

- `GITSTORE_GIT_BRANCH`

### Local source

When `CPA_IMPORT_SOURCE_DIR` is configured, startup reads:

- `{CPA_IMPORT_SOURCE_DIR}/auths/*.json`
- `{CPA_IMPORT_SOURCE_DIR}/config/config.yaml`

## Provider Mapping

Legacy provider to `sub2api` platform:

- `claude`, `anthropic` -> `anthropic`
- `codex`, `openai` -> `openai`
- `gemini`, `gemini-cli`, `aistudio`, `vertex` -> `gemini`
- `antigravity` -> `antigravity`

Unsupported providers are skipped with warnings, not fatal for the whole import.

## Account Mapping

### Account identity

- one legacy auth file -> one `sub2api` account
- stable mapping key:
  - prefer legacy auth `id`
  - fallback to filename

### Account type mapping

- has `attributes.api_key` -> `api_key`
- `vertex` + service account payload -> `service_account`
- otherwise -> `oauth`

### Credentials mapping

Imported into `accounts.credentials` where possible:

- `api_key`
- `base_url`
- `access_token`
- `refresh_token`
- `id_token`
- `email`
- `project_id`
- `location`
- `service_account`
- `user_agent`
- `oauth_type`
- `plan_type`
- `chatgpt_account_id`
- `chatgpt_user_id`
- `organization_id`
- selected raw token fields from Gemini token storage

### Extra mapping

Imported into `accounts.extra` for traceability:

- `legacy_cpa_id`
- `legacy_cpa_file`
- `legacy_cpa_provider`
- `legacy_cpa_prefix`
- `legacy_cpa_proxy`
- `legacy_cpa_label`
- `legacy_cpa_raw`

### Status mapping

- legacy `disabled=true` -> `disabled`
- otherwise -> `active`

## Proxy Mapping

When a legacy auth has `proxy_url`:

- parse URL
- reuse existing matching proxy if present
- otherwise create a new active proxy in `sub2api`
- bind the imported account to that proxy

If parsing fails, preserve the raw proxy URL in `extra` and record a warning.

## API Key Mapping

Legacy source:

- `config/config.yaml` top-level `api-keys`

Import target:

- `api_keys`

Rules:

- create under the synthetic legacy user
- do not bind group
- use `APIKeyService.Create(... CustomKey: &rawKey)`
- keep original key string unchanged

## Idempotency Model

Database tables:

- `cpa_import_runs`
- `cpa_import_mappings`

Mappings store:

- legacy account key -> target account id + checksum
- legacy api key hash -> target api key id + checksum

On restart:

- unchanged account checksum -> skip
- changed account checksum -> update account
- existing imported key -> skip

## File Changes

### New files

- `sub2api/CPA_IMPORT_PLAN.md`
- `sub2api/backend/migrations/136_cpa_import_state.sql`
- `sub2api/backend/internal/cpaimport/types.go`
- `sub2api/backend/internal/cpaimport/env.go`
- `sub2api/backend/internal/cpaimport/state_repo.go`
- `sub2api/backend/internal/cpaimport/source.go`
- `sub2api/backend/internal/cpaimport/parser.go`
- `sub2api/backend/internal/cpaimport/bootstrap_service.go`
- `sub2api/backend/internal/cpaimport/wire.go`

### Modified files

- `sub2api/backend/cmd/server/main.go`
- `sub2api/backend/cmd/server/wire.go`
- `sub2api/Dockerfile`

## Execution Plan

### Step 1

Create the plan doc and import state tables.

### Step 2

Implement `cpaimport` package:

- env parsing
- source snapshot
- auth/config parser
- provider/account mapping
- import state repo
- bootstrap runner

### Step 3

Inject bootstrap service into server startup and run it before `ListenAndServe`.

### Step 4

Regenerate wire output and fix compile issues.

### Step 5

Run verification:

- compile / tests
- import-path sanity checks

## Verification Target

- `sub2api` starts with compatibility import enabled
- old CPA auth files become `sub2api` accounts
- old CPA `api-keys` become `sub2api` API keys
- original keys stay unchanged
- repeated startup does not create duplicate accounts or keys

## Risks

### Proxy normalization

Legacy proxy strings may not always map cleanly to `sub2api` proxy records.

### Niche provider payloads

Some rare legacy auth variants may carry fields not used by the first importer.

### Manual edits on imported accounts

If operators manually edit compatibility-managed imported accounts, later import runs may overwrite some importer-managed fields.

## Follow-up After Phase 1

- add import report API / admin page
- support selective provider filters
- add one-shot bootstrap lock / switch
- add migration of more legacy provider variants
- add tests around provider-specific credential mapping
