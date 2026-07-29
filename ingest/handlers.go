package main

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

func noCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(token string, next http.HandlerFunc) http.HandlerFunc {
	if token == "" {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func handlePostWeight(db *sql.DB, callbackURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Kg  float64  `json:"kg"`
			Ohm *float64 `json:"ohm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if p.Kg < 10.0 || p.Kg > 200.0 {
			writeError(w, http.StatusBadRequest, "kg out of range (10-200)")
			return
		}

		var impedance *int
		if p.Ohm != nil {
			v := int(*p.Ohm)
			impedance = &v
		}

		record, dup, err := insertWeight(db, p.Kg, impedance)
		if err != nil {
			log.Printf("ERROR insert: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if dup {
			log.Printf("DUPLICATE weight=%.2f kg", p.Kg)
		} else if impedance != nil {
			log.Printf("STORED weight=%.2f kg impedance=%d", p.Kg, *impedance)
		} else {
			log.Printf("STORED weight=%.2f kg", p.Kg)
		}

		w.WriteHeader(http.StatusNoContent)

		if !dup && callbackURL != "" && record != nil {
			go fireCallback(callbackURL, record)
		}
	}
}

func handlePostClear(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, err := clearWeights(db)
		if err != nil {
			log.Printf("ERROR clear: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		log.Printf("CLEARED %d records", n)
		w.WriteHeader(http.StatusNoContent)
	}
}

func fireCallback(url string, record *WeightRecord) {
	body, err := json.Marshal(record)
	if err != nil {
		log.Printf("CALLBACK marshal error: %v", err)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("CALLBACK error: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("CALLBACK %s → %d", url, resp.StatusCode)
}

func handleGetWeight(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		from, err := parseDate(q.Get("from"), false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from")
			return
		}
		to, err := parseDate(q.Get("to"), true)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to")
			return
		}

		limit := 100
		if s := q.Get("limit"); s != "" {
			v, err := strconv.Atoi(s)
			if err != nil || v < 1 {
				writeError(w, http.StatusBadRequest, "invalid limit")
				return
			}
			limit = v
		}
		if limit > 1000 {
			limit = 1000
		}

		order := "desc"
		if s := q.Get("order"); s != "" {
			if s != "asc" && s != "desc" {
				writeError(w, http.StatusBadRequest, "order must be asc or desc")
				return
			}
			order = s
		}

		records, err := queryWeights(db, from, to, limit, order)
		if err != nil {
			log.Printf("ERROR query: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if records == nil {
			records = []WeightRecord{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"count": len(records),
			"items": records,
		})
	}
}

func handleGetWeightLatest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record, err := queryLatest(db)
		if err != nil {
			log.Printf("ERROR query latest: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if record == nil {
			writeError(w, http.StatusNotFound, "no data")
			return
		}
		writeJSON(w, http.StatusOK, record)
	}
}

func handleGetWeightCSV(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		from, err := parseDate(q.Get("from"), false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from")
			return
		}
		to, err := parseDate(q.Get("to"), true)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to")
			return
		}

		records, err := queryWeights(db, from, to, 0, "desc")
		if err != nil {
			log.Printf("ERROR query csv: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		w.Header().Set("Content-Type", "text/csv")
		cw := csv.NewWriter(w)
		cw.Write([]string{"measured_at", "weight_kg", "impedance", "source"})

		for _, rec := range records {
			imp := ""
			if rec.Impedance != nil {
				imp = strconv.Itoa(*rec.Impedance)
			}
			cw.Write([]string{
				rec.MeasuredAt,
				fmt.Sprintf("%.2f", rec.WeightKg),
				imp,
				rec.Source,
			})
		}
		cw.Flush()
	}
}

func handleHealthz(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			writeError(w, http.StatusServiceUnavailable, "db unreachable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseDate(s string, isTo bool) (string, error) {
	if s == "" {
		return "", nil
	}
	if len(s) == 10 {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return "", err
		}
		if isTo {
			return s + "T23:59:59Z", nil
		}
		return s + "T00:00:00Z", nil
	}
	if _, err := time.Parse(time.RFC3339, s); err != nil {
		return "", err
	}
	return s, nil
}
