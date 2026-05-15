CREATE TABLE IF NOT EXISTS cpa_import_runs (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    status TEXT NOT NULL,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS cpa_import_mappings (
    id BIGSERIAL PRIMARY KEY,
    legacy_type TEXT NOT NULL,
    legacy_id TEXT NOT NULL,
    target_kind TEXT NOT NULL,
    target_id BIGINT NOT NULL,
    checksum TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (legacy_type, legacy_id, target_kind)
);
