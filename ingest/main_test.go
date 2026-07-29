package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostWeight_Stabilized(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	handler := handlePostWeight(db, "")
	body := `{"kg":65.00,"ohm":512}`
	req := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	record, err := queryLatest(db)
	if err != nil {
		t.Fatalf("queryLatest: %v", err)
	}
	if record == nil {
		t.Fatal("expected record, got nil")
	}
	if record.WeightKg != 65.00 {
		t.Errorf("weight: expected 65.00, got %.2f", record.WeightKg)
	}
	if record.Impedance == nil || *record.Impedance != 512 {
		t.Errorf("impedance: expected 512, got %v", record.Impedance)
	}
	if record.Source != "esp32" {
		t.Errorf("source: expected esp32, got %s", record.Source)
	}
}

func TestPostWeight_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	handler := handlePostWeight(db, "")

	cases := []struct {
		name string
		body string
	}{
		{"too_light", `{"kg":5.0,"ohm":512}`},
		{"too_heavy", `{"kg":250.0,"ohm":512}`},
		{"zero", `{"kg":0}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
		})
	}

	record, err := queryLatest(db)
	if err != nil {
		t.Fatalf("queryLatest: %v", err)
	}
	if record != nil {
		t.Error("expected no records stored")
	}
}

func TestPostWeight_NoImpedance(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	handler := handlePostWeight(db, "")
	body := `{"kg":65.00}`
	req := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	record, err := queryLatest(db)
	if err != nil {
		t.Fatalf("queryLatest: %v", err)
	}
	if record == nil {
		t.Fatal("expected record, got nil")
	}
	if record.WeightKg != 65.00 {
		t.Errorf("weight: expected 65.00, got %.2f", record.WeightKg)
	}
	if record.Impedance != nil {
		t.Errorf("impedance: expected nil, got %v", *record.Impedance)
	}
}

func TestPostWeight_Duplicate(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	handler := handlePostWeight(db, "")
	body := `{"kg":65.00,"ohm":512}`

	req1 := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(body))
	rec1 := httptest.NewRecorder()
	handler(rec1, req1)
	if rec1.Code != http.StatusNoContent {
		t.Fatalf("first insert: expected 204, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(body))
	rec2 := httptest.NewRecorder()
	handler(rec2, req2)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("duplicate insert: expected 204, got %d", rec2.Code)
	}

	records, err := queryWeights(db, "", "", 0, "desc")
	if err != nil {
		t.Fatalf("queryWeights: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record after duplicate, got %d", len(records))
	}
}

func TestGetWeightLatest_Empty(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	handler := handleGetWeightLatest(db)
	req := httptest.NewRequest(http.MethodGet, "/weight/latest", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetWeight_ListResponse(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	postHandler := handlePostWeight(db, "")
	req := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(`{"kg":70.00}`))
	rec := httptest.NewRecorder()
	postHandler(rec, req)

	getHandler := handleGetWeight(db)
	req = httptest.NewRequest(http.MethodGet, "/weight", nil)
	rec = httptest.NewRecorder()
	getHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Count int            `json:"count"`
		Items []WeightRecord `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Count)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Items))
	}
}

func TestPostClear(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	post := handlePostWeight(db, "")
	req := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(`{"kg":70.00}`))
	rec := httptest.NewRecorder()
	post(rec, req)

	record, _ := queryLatest(db)
	if record == nil {
		t.Fatal("expected record before clear")
	}

	clear := handlePostClear(db)
	req = httptest.NewRequest(http.MethodPost, "/weight/clear", nil)
	rec = httptest.NewRecorder()
	clear(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}

	record, _ = queryLatest(db)
	if record != nil {
		t.Error("expected no records after clear")
	}
}

func TestCallback(t *testing.T) {
	var called bool
	var callbackBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		json.NewDecoder(r.Body).Decode(&callbackBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	handler := handlePostWeight(db, srv.URL)
	req := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(`{"kg":72.50,"ohm":440}`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}

	for i := 0; i < 50; i++ {
		if called {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !called {
		t.Fatal("callback was not called")
	}
	if callbackBody["weight_kg"] != 72.5 {
		t.Errorf("callback weight: expected 72.5, got %v", callbackBody["weight_kg"])
	}
	if callbackBody["source"] != "esp32" {
		t.Errorf("callback source: expected esp32, got %v", callbackBody["source"])
	}
}

func TestCallback_NotCalledOnDuplicate(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	handler := handlePostWeight(db, srv.URL)

	req1 := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(`{"kg":72.50}`))
	rec1 := httptest.NewRecorder()
	handler(rec1, req1)

	time.Sleep(100 * time.Millisecond)

	req2 := httptest.NewRequest(http.MethodPost, "/weight", strings.NewReader(`{"kg":72.50}`))
	rec2 := httptest.NewRecorder()
	handler(rec2, req2)

	time.Sleep(100 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("expected callback called once, got %d", callCount)
	}
}

func TestHealthz(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	handler := handleHealthz(db)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
