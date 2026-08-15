package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
)

var ErrRevisionConflict = errors.New("configuration revision conflict")

type Revision struct {
	ID              int64           `json:"id"`
	Hash            string          `json:"hash"`
	RestartRequired bool            `json:"restart_required"`
	Actor           string          `json:"actor,omitempty"`
	Before          config.Redacted `json:"before"`
	After           config.Redacted `json:"after"`
	CreatedAt       time.Time       `json:"created_at,omitempty"`
}

type Store struct {
	path    string
	queries *db.Queries
}

func New(path string, queries *db.Queries) *Store        { return &Store{path: path, queries: queries} }
func (s *Store) Path() string                            { return s.path }
func (s *Store) Validate(editable config.Editable) error { return config.ValidateEditable(editable) }

func (s *Store) current() (*config.Config, string, error) {
	cfg, configured, err := config.LoadOptional(s.path)
	if err != nil {
		return nil, "", err
	}
	if !configured {
		return config.FromEditable(config.Editable{}), "", nil
	}
	return cfg, config.EditableHash(cfg.Editable()), nil
}

func (s *Store) Save(ctx context.Context, actor, expectedHash string, editable config.Editable) (Revision, error) {
	if err := s.Validate(editable); err != nil {
		return Revision{}, err
	}
	current, currentHash, err := s.current()
	if err != nil {
		return Revision{}, err
	}
	if expectedHash != currentHash {
		return Revision{}, ErrRevisionConflict
	}

	data, err := json.MarshalIndent(editable, "", "  ")
	if err != nil {
		return Revision{}, err
	}
	data = append(data, '\n')
	if err := atomicWrite(s.path, data); err != nil {
		return Revision{}, err
	}

	afterCfg := config.FromEditable(editable)
	before := config.Redact(current)
	after := config.Redact(afterCfg)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	hash := config.EditableHash(editable)
	id, err := s.queries.InsertConfigRevision(ctx, db.InsertConfigRevisionParams{
		ActorAddress: sql.NullString{String: actor, Valid: strings.TrimSpace(actor) != ""},
		BeforeJson:   sql.NullString{String: string(beforeJSON), Valid: true}, AfterJson: sql.NullString{String: string(afterJSON), Valid: true},
		ValidationState: sql.NullString{String: "valid", Valid: true}, WriteState: sql.NullString{String: "written", Valid: true}, ConfigHash: sql.NullString{String: hash, Valid: true},
	})
	if err != nil {
		return Revision{}, fmt.Errorf("record config revision: %w", err)
	}
	return Revision{ID: id, Hash: hash, RestartRequired: true, Actor: actor, Before: before, After: after}, nil
}

func (s *Store) ListRevisions(ctx context.Context) ([]Revision, error) {
	rows, err := s.queries.ListConfigRevisions(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Revision, 0, len(rows))
	for _, row := range rows {
		var before, after config.Redacted
		_ = json.Unmarshal([]byte(row.BeforeJson.String), &before)
		_ = json.Unmarshal([]byte(row.AfterJson.String), &after)
		revision := Revision{ID: row.ID, Hash: row.ConfigHash.String, RestartRequired: !row.ActivatedAt.Valid, Actor: row.ActorAddress.String, Before: before, After: after}
		if row.CreatedAt.Valid {
			revision.CreatedAt = row.CreatedAt.Time
		}
		result = append(result, revision)
	}
	return result, nil
}

func (s *Store) Restore(ctx context.Context, actor, expectedHash string, id int64) (Revision, error) {
	row, err := s.queries.GetConfigRevision(ctx, id)
	if err != nil {
		return Revision{}, err
	}
	var snapshot config.Redacted
	if err := json.Unmarshal([]byte(row.AfterJson.String), &snapshot); err != nil {
		return Revision{}, err
	}
	return s.Save(ctx, actor, expectedHash, snapshot.Settings)
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".gopool-config-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
