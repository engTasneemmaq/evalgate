package gate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var ErrNotFound = errors.New("run or reviewable case not found")

type Store struct {
	mu   sync.Mutex
	path string
	db   *sql.DB
}

func OpenStore(ctx context.Context, databaseURL, path string) (*Store, error) {
	store := &Store{path: path}
	if databaseURL == "" {
		return store, nil
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS evaluation_runs (
		id text PRIMARY KEY, created_at timestamptz NOT NULL, data jsonb NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	store.db = db
	return store, nil
}

func (s *Store) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

func (s *Store) readFile() ([]Run, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Run{}, nil
	}
	if err != nil {
		return nil, err
	}
	var runs []Run
	err = json.Unmarshal(data, &runs)
	return runs, err
}

func (s *Store) writeFile(runs []Run) error {
	data, err := json.Marshal(runs)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".runs-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err = errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.path)
}

func (s *Store) List(ctx context.Context) ([]Run, error) {
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.readFile()
	}
	rows, err := s.db.QueryContext(ctx, "SELECT data FROM evaluation_runs ORDER BY created_at DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var data []byte
		var run Run
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &run); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) Add(ctx context.Context, run Run) error {
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		runs, err := s.readFile()
		if err != nil {
			return err
		}
		return s.writeFile(append([]Run{run}, runs...))
	}
	data, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO evaluation_runs VALUES ($1, $2, $3)", run.ID, run.CreatedAt, data)
	return err
}

func review(run *Run, caseID, decision string) error {
	for i := range run.Results {
		if run.Results[i].Case.ID == caseID && run.Results[i].NeedsReview {
			run.Results[i].Review = decision
			run.UpdateStatus()
			return nil
		}
	}
	return ErrNotFound
}

func (s *Store) Review(ctx context.Context, id, caseID, decision string) (Run, error) {
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		runs, err := s.readFile()
		if err != nil {
			return Run{}, err
		}
		for i := range runs {
			if runs[i].ID == id {
				if err = review(&runs[i], caseID, decision); err != nil {
					return Run{}, err
				}
				return runs[i], s.writeFile(runs)
			}
		}
		return Run{}, ErrNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	var data []byte
	err = tx.QueryRowContext(ctx, "SELECT data FROM evaluation_runs WHERE id = $1 FOR UPDATE", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err = json.Unmarshal(data, &run); err != nil {
		return Run{}, err
	}
	if err = review(&run, caseID, decision); err != nil {
		return Run{}, err
	}
	data, err = json.Marshal(run)
	if err != nil {
		return Run{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE evaluation_runs SET data = $2 WHERE id = $1", id, data); err != nil {
		return Run{}, err
	}
	return run, tx.Commit()
}
