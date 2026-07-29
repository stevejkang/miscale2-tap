package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	addr := getenv("LISTEN_ADDR", ":51001")
	dbPath := getenv("DB_PATH", "/data/weight.db")
	apiToken := os.Getenv("API_TOKEN")
	callbackURL := os.Getenv("CALLBACK_URL")

	db, err := openDB(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /weight", handlePostWeight(db, callbackURL))
	mux.HandleFunc("POST /weight/clear", authMiddleware(apiToken, handlePostClear(db)))
	mux.HandleFunc("GET /weight", authMiddleware(apiToken, handleGetWeight(db)))
	mux.HandleFunc("GET /weight/latest", authMiddleware(apiToken, handleGetWeightLatest(db)))
	mux.HandleFunc("GET /weight.csv", authMiddleware(apiToken, handleGetWeightCSV(db)))
	mux.HandleFunc("GET /healthz", handleHealthz(db))

	handler := noCacheMiddleware(mux)

	log.Printf("listening on %s (db=%s)", addr, dbPath)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
