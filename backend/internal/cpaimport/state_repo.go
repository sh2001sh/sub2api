package cpaimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type StateRepo struct {
	db *sql.DB
}

// NewStateRepo creates a repository for CPA import state persistence.
func NewStateRepo(db *sql.DB) *StateRepo {
	return &StateRepo{db: db}
}

func (r *StateRepo) BeginRun(ctx context.Context, source string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO cpa_import_runs (source, status) VALUES ($1, $2) RETURNING id`,
		source,
		"running",
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert cpa import run: %w", err)
	}
	return id, nil
}

func (r *StateRepo) FinishRun(ctx context.Context, runID int64, status string, summary any, runErr error) error {
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal cpa import summary: %w", err)
	}
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	_, err = r.db.ExecContext(
		ctx,
		`UPDATE cpa_import_runs SET status = $2, summary = $3::jsonb, error = $4, finished_at = NOW() WHERE id = $1`,
		runID,
		status,
		string(summaryJSON),
		errText,
	)
	if err != nil {
		return fmt.Errorf("update cpa import run: %w", err)
	}
	return nil
}

func (r *StateRepo) GetMapping(ctx context.Context, legacyType, legacyID, targetKind string) (*ImportMapping, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT legacy_type, legacy_id, target_kind, target_id, checksum
		 FROM cpa_import_mappings
		 WHERE legacy_type = $1 AND legacy_id = $2 AND target_kind = $3`,
		legacyType,
		legacyID,
		targetKind,
	)
	var mapping ImportMapping
	if err := row.Scan(&mapping.LegacyType, &mapping.LegacyID, &mapping.TargetKind, &mapping.TargetID, &mapping.Checksum); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query cpa import mapping: %w", err)
	}
	return &mapping, nil
}

func (r *StateRepo) UpsertMapping(ctx context.Context, mapping ImportMapping) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO cpa_import_mappings (legacy_type, legacy_id, target_kind, target_id, checksum)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (legacy_type, legacy_id, target_kind)
		 DO UPDATE SET target_id = EXCLUDED.target_id, checksum = EXCLUDED.checksum, updated_at = NOW()`,
		mapping.LegacyType,
		mapping.LegacyID,
		mapping.TargetKind,
		mapping.TargetID,
		mapping.Checksum,
	)
	if err != nil {
		return fmt.Errorf("upsert cpa import mapping: %w", err)
	}
	return nil
}
