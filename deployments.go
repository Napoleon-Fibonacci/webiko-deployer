package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Status vocabulary; must stay in sync with the CHECK in db/schema.sql.
var deployStatuses = []string{"pending", "active", "deleting", "deleted", "failed"}

var statusLabel = map[string]string{
	"pending":  "Menunggu",
	"active":   "Aktif",
	"deleting": "Menghapus",
	"deleted":  "Terhapus",
	"failed":   "Gagal",
}

// A live row uses a slot and locks its name. Stale pending rows do not, so
// correctness never depends on how often the cron actually runs.
const liveSQL = `(status IN ('active','deleting') OR (status = 'pending' AND created_at > now() - interval '600 seconds'))`

const failStaleSQL = `UPDATE deployments SET status = 'failed'
  WHERE status = 'pending' AND created_at <= now() - interval '600 seconds'`

const pruneRateLimitsSQL = `DELETE FROM rate_limits
  WHERE window_start < now() - interval '1 day'`

// Rows past their TTL leave "active" first, then the sweep below deletes the
// Vercel project. Marking them separately keeps a slow Vercel call from losing
// track of which sites have already expired.
const expireOverdueSQL = `UPDATE deployments SET status = 'deleting'
  WHERE id IN (
    SELECT id FROM deployments
    WHERE status = 'active' AND created_at < $1
    ORDER BY created_at ASC
    LIMIT $2
  )`

type ReservedVictim struct {
	ID              string
	Name            string
	VercelProjectID sql.NullString
}

type Reservation struct {
	RowID  string
	Victim *ReservedVictim
}

type ActiveRow struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type AdminRow struct {
	ID         string
	Name       string
	Status     string
	SourceType string
	URL        sql.NullString
	CreatedAt  time.Time
	DeletedAt  sql.NullTime
}

type AdminFilter struct {
	Status string
	Query  string
}

type AdminList struct {
	Rows    []AdminRow
	Counts  map[string]int
	Total   int
	Matched int
}

type SweepResult struct {
	Failed    int64 `json:"failed"`
	Expired   int64 `json:"expired"`
	Deleted   int   `json:"deleted"`
	Gone      int   `json:"gone"`
	Blobs     int   `json:"blobs"`
	Remaining int   `json:"remaining"`
}

type DeleteOneResult struct {
	Name    string
	Outcome string // "done" or "retry"
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// reserveSlot creates the pending row under an advisory transaction lock and
// marks the oldest live project for deletion when the cap is exceeded.
func reserveSlot(name, sourceType string) (Reservation, error) {
	db, err := getDB()
	if err != nil {
		return Reservation{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(1)"); err != nil {
		return Reservation{}, err
	}
	if _, err = tx.ExecContext(ctx, failStaleSQL); err != nil {
		return Reservation{}, err
	}

	var rowID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO deployments (name, source_type, status) VALUES ($1, $2, 'pending') RETURNING id`,
		name, sourceType,
	).Scan(&rowID)
	if err != nil {
		if isUniqueViolation(err) {
			return Reservation{}, newNameTaken()
		}
		return Reservation{}, err
	}

	var count int
	if err = tx.QueryRowContext(ctx,
		`SELECT count(*)::int FROM deployments WHERE `+liveSQL,
	).Scan(&count); err != nil {
		return Reservation{}, err
	}

	var victim *ReservedVictim
	if count > maxProjects() {
		// Expired sites go first even if a younger site is technically the
		// global oldest; they were already due for removal anyway.
		cutoff := time.Now().Add(-siteTTL())
		row := tx.QueryRowContext(ctx,
			`UPDATE deployments SET status = 'deleting'
			 WHERE id = (
			   SELECT id FROM deployments
			   WHERE `+liveSQL+` AND status <> 'deleting'
			   ORDER BY (status = 'active' AND created_at < $1) DESC, created_at ASC
			   LIMIT 1
			   FOR UPDATE SKIP LOCKED
			 )
			 RETURNING id, name, vercel_project_id`, cutoff)
		var found ReservedVictim
		if err = row.Scan(&found.ID, &found.Name, &found.VercelProjectID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Reservation{}, errors.New("batas tercapai tapi tidak ada project hidup untuk dihapus")
			}
			return Reservation{}, err
		}
		victim = &found
	}

	if err = tx.Commit(); err != nil {
		return Reservation{}, err
	}
	return Reservation{RowID: rowID, Victim: victim}, nil
}

func markActive(rowID, vercelProjectID, publicURL string) error {
	return execQuery(`UPDATE deployments SET status = 'active', vercel_project_id = $2, url = $3 WHERE id = $1`,
		rowID, vercelProjectID, publicURL)
}

func markFailed(rowID string) error {
	return execQuery(`UPDATE deployments SET status = 'failed' WHERE id = $1`, rowID)
}

func markDeleted(rowID string) error {
	return execQuery(`UPDATE deployments SET status = 'deleted', deleted_at = now() WHERE id = $1`, rowID)
}

func revertVictim(rowID string) error {
	return execQuery(`UPDATE deployments SET status = 'active' WHERE id = $1 AND status = 'deleting'`, rowID)
}

func execQuery(query string, args ...any) error {
	db, err := getDB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, query, args...)
	return err
}

// deleteOne removes a single row from the admin menu: deleting -> Vercel ->
// deleted. A Vercel refusal leaves it "deleting" so the sweep can retry.
func deleteOne(rowID string) (DeleteOneResult, error) {
	db, err := getDB()
	if err != nil {
		return DeleteOneResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var row struct {
		Name            string
		Status          string
		VercelProjectID sql.NullString
	}
	err = db.QueryRowContext(ctx,
		`SELECT name, status, vercel_project_id FROM deployments WHERE id = $1`, rowID,
	).Scan(&row.Name, &row.Status, &row.VercelProjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return DeleteOneResult{}, newAppError(404, "Baris tidak ditemukan.", FieldSource)
	}
	if err != nil {
		return DeleteOneResult{}, err
	}
	if row.Status == "deleted" {
		return DeleteOneResult{Name: row.Name, Outcome: "done"}, nil
	}
	if err = execQuery(`UPDATE deployments SET status = 'deleting' WHERE id = $1`, rowID); err != nil {
		return DeleteOneResult{}, err
	}
	if row.VercelProjectID.Valid && row.VercelProjectID.String != "" {
		if err = deleteVercelProject(row.VercelProjectID.String); err != nil {
			return DeleteOneResult{Name: row.Name, Outcome: "retry"}, nil
		}
	}
	if err = markDeleted(rowID); err != nil {
		return DeleteOneResult{}, err
	}
	return DeleteOneResult{Name: row.Name, Outcome: "done"}, nil
}

func countLive() (int, error) {
	db, err := getDB()
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	err = db.QueryRowContext(ctx, `SELECT count(*)::int FROM deployments WHERE `+liveSQL).Scan(&count)
	return count, err
}

func findLiveByName(name string) (bool, error) {
	db, err := getDB()
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var found int
	err = db.QueryRowContext(ctx,
		`SELECT 1 FROM deployments WHERE name = $1 AND `+liveSQL+` LIMIT 1`, name,
	).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func recentActive(limit int) ([]ActiveRow, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 20
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, created_at FROM deployments
		 WHERE status IN ('active','deleting')
		 ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActiveRow
	for rows.Next() {
		var row ActiveRow
		if err = rows.Scan(&row.ID, &row.Name, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func adminWhere(filter AdminFilter) (string, []any) {
	var conditions []string
	var values []any
	if filter.Status != "" {
		values = append(values, filter.Status)
		conditions = append(conditions, "status = $"+itoa(len(values)))
	}
	if search := strings.TrimSpace(filter.Query); search != "" {
		values = append(values, "%"+search+"%")
		conditions = append(conditions, "name ILIKE $"+itoa(len(values)))
	}
	if len(conditions) == 0 {
		return "", values
	}
	return "WHERE " + strings.Join(conditions, " AND "), values
}

func listForAdmin(filter AdminFilter, limit int) (AdminList, error) {
	db, err := getDB()
	if err != nil {
		return AdminList{}, err
	}
	if limit < 1 {
		limit = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	clause, values := adminWhere(filter)
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, status, source_type, url, created_at, deleted_at
		 FROM deployments `+clause+` ORDER BY created_at DESC LIMIT $`+itoa(len(values)+1),
		append(values, limit)...)
	if err != nil {
		return AdminList{}, err
	}
	defer rows.Close()

	list := AdminList{Counts: map[string]int{}}
	for _, key := range deployStatuses {
		list.Counts[key] = 0
	}
	for rows.Next() {
		var row AdminRow
		if err = rows.Scan(&row.ID, &row.Name, &row.Status, &row.SourceType, &row.URL, &row.CreatedAt, &row.DeletedAt); err != nil {
			return AdminList{}, err
		}
		list.Rows = append(list.Rows, row)
	}
	if err = rows.Err(); err != nil {
		return AdminList{}, err
	}

	perStatus, err := db.QueryContext(ctx, `SELECT status, count(*)::int FROM deployments GROUP BY status`)
	if err != nil {
		return AdminList{}, err
	}
	defer perStatus.Close()
	for perStatus.Next() {
		var status string
		var count int
		if err = perStatus.Scan(&status, &count); err != nil {
			return AdminList{}, err
		}
		list.Counts[status] = count
	}
	if err = perStatus.Err(); err != nil {
		return AdminList{}, err
	}
	for _, key := range deployStatuses {
		list.Total += list.Counts[key]
	}

	if err = db.QueryRowContext(ctx,
		`SELECT count(*)::int FROM deployments `+clause, values...,
	).Scan(&list.Matched); err != nil {
		return AdminList{}, err
	}
	return list, nil
}

// reconcileGone closes rows that claim to be live while the project is gone
// from Vercel. Only a confident 404 counts; other answers stay unknown.
func reconcileGone(limit int) (int, error) {
	db, err := getDB()
	if err != nil {
		return 0, err
	}
	if limit < 1 {
		limit = maxProjects()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT id, vercel_project_id FROM deployments
		 WHERE status = 'active' AND vercel_project_id IS NOT NULL
		 ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type pair struct{ ID, ProjectID string }
	var items []pair
	for rows.Next() {
		var item pair
		var projectID sql.NullString
		if err = rows.Scan(&item.ID, &projectID); err != nil {
			rows.Close()
			return 0, err
		}
		if projectID.Valid {
			item.ProjectID = projectID.String
			items = append(items, item)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	gone := 0
	const concurrency = 12
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(item pair) {
			defer wg.Done()
			defer func() { <-sem }()
			exists, checkErr := projectExists(item.ProjectID)
			if checkErr != nil || exists == nil || *exists {
				return
			}
			if markDeleted(item.ID) == nil {
				mu.Lock()
				gone++
				mu.Unlock()
			}
		}(item)
	}
	wg.Wait()
	return gone, nil
}

// expireOverdue moves sites past their TTL into the deletion queue. The limit
// keeps one run bounded; the cron asks for a larger batch than the admin button.
// Leftover expired rows are still preferred victims in reserveSlot.
func expireOverdue(cutoff time.Time, limit int) (int64, error) {
	db, err := getDB()
	if err != nil {
		return 0, err
	}
	if limit < 1 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := db.ExecContext(ctx, expireOverdueSQL, cutoff, limit)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

// sweepStale is the safety net used by the daily cron and the admin button.
func sweepStale(limit int) (SweepResult, error) {
	db, err := getDB()
	if err != nil {
		return SweepResult{}, err
	}
	if limit < 1 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), 115*time.Second)
	defer cancel()

	if _, err = db.ExecContext(ctx, pruneRateLimitsSQL); err != nil {
		return SweepResult{}, err
	}
	failed, err := db.ExecContext(ctx, failStaleSQL)
	if err != nil {
		return SweepResult{}, err
	}
	affected, _ := failed.RowsAffected()

	// Queue overdue sites in the same batch size the Vercel deletes can clear.
	// Any leftover expired rows stay "active" but reserveSlot still picks them
	// first, so a backlog never pushes a younger site out.
	expired, err := expireOverdue(time.Now().Add(-siteTTL()), limit)
	if err != nil {
		return SweepResult{}, err
	}

	rows, err := db.QueryContext(ctx,
		`SELECT id, vercel_project_id FROM deployments
		 WHERE status = 'deleting' ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return SweepResult{}, err
	}
	type pendingDeletion struct {
		ID        string
		ProjectID string
	}
	var pending []pendingDeletion
	for rows.Next() {
		var item pendingDeletion
		var projectID sql.NullString
		if err = rows.Scan(&item.ID, &projectID); err != nil {
			rows.Close()
			return SweepResult{}, err
		}
		if projectID.Valid {
			item.ProjectID = projectID.String
		}
		pending = append(pending, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return SweepResult{}, err
	}
	rows.Close()

	deleted := 0
	for _, item := range pending {
		if item.ProjectID != "" {
			if err = deleteVercelProject(item.ProjectID); err != nil {
				continue
			}
		}
		if markDeleted(item.ID) == nil {
			deleted++
		}
	}

	gone, err := reconcileGone(maxProjects())
	if err != nil {
		return SweepResult{}, err
	}

	// Abandoned ZIPs are cheap to remove and should never linger in Blob
	// storage. Failures here must not fail the whole sweep.
	blobs, blobErr := cleanupOrphanBlobs(24 * time.Hour)
	if blobErr != nil {
		log.Printf("cleanup orphan blobs: %v", blobErr)
	}

	var remaining int
	if err = db.QueryRowContext(ctx, `SELECT count(*)::int FROM deployments WHERE status = 'deleting'`).Scan(&remaining); err != nil {
		return SweepResult{}, err
	}
	return SweepResult{
		Failed:    affected,
		Expired:   expired,
		Deleted:   deleted,
		Gone:      gone,
		Blobs:     blobs,
		Remaining: remaining,
	}, nil
}
