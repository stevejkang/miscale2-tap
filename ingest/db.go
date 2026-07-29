package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type WeightRecord struct {
	MeasuredAt string  `json:"measured_at"`
	WeightKg   float64 `json:"weight_kg"`
	Impedance  *int    `json:"impedance"`
	Source     string  `json:"source"`
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return db, nil
}

func insertWeight(db *sql.DB, kg float64, impedance *int) (record *WeightRecord, duplicate bool, err error) {
	now := time.Now().UTC()
	measuredAt := now.Format(time.RFC3339)
	bucket := now.Truncate(time.Minute).Format(time.RFC3339)

	res, err := db.Exec(
		`INSERT INTO weight (measured_at, bucket, weight_kg, impedance)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (bucket, weight_kg) DO NOTHING`,
		measuredAt, bucket, kg, impedance,
	)
	if err != nil {
		return nil, false, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if n == 0 {
		return nil, true, nil
	}

	return &WeightRecord{
		MeasuredAt: measuredAt,
		WeightKg:   kg,
		Impedance:  impedance,
		Source:     "esp32",
	}, false, nil
}

func queryWeights(db *sql.DB, from, to string, limit int, order string) ([]WeightRecord, error) {
	q := "SELECT measured_at, weight_kg, impedance, source FROM weight"
	var conds []string
	var args []any

	if from != "" {
		conds = append(conds, "measured_at >= ?")
		args = append(args, from)
	}
	if to != "" {
		conds = append(conds, "measured_at <= ?")
		args = append(args, to)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}

	if order == "asc" {
		q += " ORDER BY measured_at ASC"
	} else {
		q += " ORDER BY measured_at DESC"
	}

	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []WeightRecord
	for rows.Next() {
		var r WeightRecord
		var imp sql.NullInt64
		if err := rows.Scan(&r.MeasuredAt, &r.WeightKg, &imp, &r.Source); err != nil {
			return nil, err
		}
		if imp.Valid {
			v := int(imp.Int64)
			r.Impedance = &v
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func clearWeights(db *sql.DB) (int64, error) {
	res, err := db.Exec("DELETE FROM weight")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func queryLatest(db *sql.DB) (*WeightRecord, error) {
	var r WeightRecord
	var imp sql.NullInt64
	err := db.QueryRow(
		"SELECT measured_at, weight_kg, impedance, source FROM weight ORDER BY measured_at DESC LIMIT 1",
	).Scan(&r.MeasuredAt, &r.WeightKg, &imp, &r.Source)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if imp.Valid {
		v := int(imp.Int64)
		r.Impedance = &v
	}
	return &r, nil
}
