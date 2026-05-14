package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"trove/backend/internal/data"
	"trove/backend/internal/handlers"
)

var version = "dev"

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS, POST, PUT, PATCH, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://trove:password@localhost:5434/trove?sslmode=disable"
	}

	db, err := data.InitDB(dbURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	supabaseURL := os.Getenv("SUPABASE_URL")
	if supabaseURL == "" {
		log.Fatal("SUPABASE_URL is required")
	}

	verifier, err := handlers.InitAuth(context.Background(), supabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize OIDC: %v", err)
	}

	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return corsMiddleware(handlers.AuthMiddleware(verifier, db, h))
	}

	projectsHandler := handlers.NewProjectsHandler(db)
	itemsHandler := handlers.NewItemsHandler(db)
	tagsHandler := handlers.NewTagsHandler(db)

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/health", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
	}))

	mux.HandleFunc("/v1/me", authed(handlers.HandleMe))

	mux.HandleFunc("/v1/projects", authed(projectsHandler.HandleCollection))
	mux.HandleFunc("/v1/projects/check-slug", authed(projectsHandler.HandleCheckSlug))
	mux.HandleFunc("/v1/projects/{slug}/items", authed(itemsHandler.HandleCollection))
	mux.HandleFunc("/v1/projects/{slug}/items/{seq}", authed(itemsHandler.HandleByID))
	mux.HandleFunc("/v1/projects/{slug}/items/{seq}/tags", authed(itemsHandler.HandleItemTags))
	mux.HandleFunc("/v1/projects/{slug}/items/{seq}/tags/{tagSlug}", authed(itemsHandler.HandleItemTagByID))
	mux.HandleFunc("/v1/projects/{slug}/tags", authed(tagsHandler.HandleTagsForProject))
	mux.HandleFunc("/v1/projects/", authed(projectsHandler.HandleByID))

	mux.HandleFunc("/v1/tags", authed(tagsHandler.HandleCollection))
	mux.HandleFunc("/v1/tags/check-slug", authed(tagsHandler.HandleCheckSlug))
	mux.HandleFunc("/v1/tags/{slug}/items", authed(tagsHandler.HandleItemsForTag))
	mux.HandleFunc("/v1/tags/", authed(tagsHandler.HandleByID))

	// TODO: register remaining handlers — groups, attachments, activity, events (SSE).

	mux.HandleFunc("/", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Not found"})
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Trove backend starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
