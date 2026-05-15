package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	photopicker "github.com/samrford/google-photos-picker"
	ppg "github.com/samrford/google-photos-picker/postgres"

	"trove/backend/internal/data"
	"trove/backend/internal/data/storage"
	"trove/backend/internal/handlers"
	"trove/backend/internal/jobs"
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

	// Object storage - Optional: if env vars
	// are missing, the server still boots but attachments are disabled
	// gracefully (handlers stay registered but item responses just won't
	// include URLs for objects we can't sign).
	var store storage.FileStore
	if endpoint := os.Getenv("STORAGE_ENDPOINT"); endpoint != "" {
		s, err := storage.InitStorage(storage.Config{
			Endpoint:        endpoint,
			AccessKeyID:     os.Getenv("STORAGE_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("STORAGE_SECRET_ACCESS_KEY"),
			Bucket:          os.Getenv("STORAGE_BUCKET"),
			Region:          os.Getenv("STORAGE_REGION"),
			UsePathStyle:    strings.ToLower(os.Getenv("STORAGE_USE_PATH_STYLE")) != "false",
		})
		if err != nil {
			log.Fatalf("Failed to initialize storage: %v", err)
		}
		store = s
		log.Println("Object storage initialized")
	} else {
		log.Println("STORAGE_ENDPOINT unset — attachments disabled")
	}

	projectsHandler := handlers.NewProjectsHandler(db)
	itemsHandler := handlers.NewItemsHandler(db, store)
	tagsHandler := handlers.NewTagsHandler(db)
	attachmentsHandler := handlers.NewAttachmentsHandler(db, store)

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

	if store != nil {
		mux.HandleFunc("/v1/projects/{slug}/items/{seq}/attachments", authed(attachmentsHandler.HandleCollection))
		mux.HandleFunc("/v1/projects/{slug}/items/{seq}/attachments/{id}", authed(attachmentsHandler.HandleByID))

		// Daily orphan sweep — Postgres advisory lock makes this safe under
		// multi-instance deploys.
		sweepCtx, cancelSweep := context.WithCancel(context.Background())
		defer cancelSweep()
		go jobs.RunOrphanSweep(sweepCtx, db, store)
	}

	// Google Photos picker — optional, only activates if all four env vars
	// are set *and* storage is available (the sink writes attachments to
	// storage, so without storage it can't work).
	googlePhotosEnabled := false
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	googleRedirectURL := os.Getenv("GOOGLE_REDIRECT_URL")
	googleEncKey := os.Getenv("GOOGLE_TOKEN_ENCRYPTION_KEY")
	if store != nil && googleClientID != "" && googleClientSecret != "" && googleRedirectURL != "" && googleEncKey != "" {
		if err := ppg.Migrate(db); err != nil {
			log.Fatalf("photopicker migrate: %v", err)
		}
		tokenStore, err := ppg.NewTokenStore(db, googleEncKey)
		if err != nil {
			log.Fatalf("photopicker token store: %v", err)
		}
		importStore := ppg.NewImportStore(db)
		client, err := photopicker.New(photopicker.Config{
			OAuth:           photopicker.NewOAuthConfig(googleClientID, googleClientSecret, googleRedirectURL),
			TokenStore:      tokenStore,
			ImportStore:     importStore,
			Sink:            handlers.NewTroveSink(db, store),
			MaxDecodedBytes: 100 << 20, // 100MB per photo — Google Photos can be chunky
		})
		if err != nil {
			log.Fatalf("photopicker: %v", err)
		}
		pp, err := photopicker.NewHandlers(photopicker.HandlersConfig{
			Client: client,
			ResolveUserID: func(r *http.Request) (string, error) {
				uid := handlers.GetUserID(r.Context())
				if uid == "" {
					return "", errors.New("unauthenticated")
				}
				return uid, nil
			},
			Callback: photopicker.CallbackPage{
				PostMessageType: "trove:google-oauth",
				TargetOrigin:    os.Getenv("FRONTEND_ORIGIN"),
			},
		})
		if err != nil {
			log.Fatalf("photopicker handlers: %v", err)
		}
		worker, err := photopicker.NewWorker(photopicker.WorkerConfig{Client: client})
		if err != nil {
			log.Fatalf("photopicker worker: %v", err)
		}
		workerCtx, cancelWorker := context.WithCancel(context.Background())
		defer cancelWorker()
		go worker.Run(workerCtx)

		// Picker-owned routes: OAuth dance + session/import polling.
		mux.HandleFunc("/v1/google-photos/connect", authed(pp.Connect()))
		mux.HandleFunc("/v1/google-photos/callback", corsMiddleware(pp.Callback())) // no auth: browser redirect target
		mux.HandleFunc("/v1/google-photos/status", authed(pp.Status()))
		mux.HandleFunc("/v1/google-photos/disconnect", authed(pp.Disconnect()))
		mux.HandleFunc("/v1/google-photos/imports/{jobID}", authed(pp.GetImport(func(r *http.Request) string {
			return r.PathValue("jobID")
		})))

		// Trove-scoped routes — capture target item, then delegate.
		gphotosHandler := handlers.NewGPhotosHandler(db, store, pp, client)
		mux.HandleFunc("/v1/projects/{slug}/items/{seq}/google-photos/sessions",
			authed(gphotosHandler.HandleCreateSession))
		mux.HandleFunc("/v1/projects/{slug}/items/{seq}/google-photos/sessions/{sid}/import",
			authed(gphotosHandler.HandleStartImport))
		mux.HandleFunc("/v1/projects/{slug}/items/{seq}/google-photos/sessions/{sid}",
			authed(pp.PollSession(func(r *http.Request) string {
				return r.PathValue("sid")
			})))

		googlePhotosEnabled = true
		log.Println("Google Photos integration enabled")
	} else {
		log.Println("Google Photos integration disabled (missing storage or env vars)")
	}

	// Lightweight feature-flag endpoint — frontend reads this to decide which
	// optional integrations to render. Cheap and unauthenticated.
	mux.HandleFunc("/v1/config", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"version":              version,
			"attachmentsEnabled":   store != nil,
			"googlePhotosEnabled":  googlePhotosEnabled,
			"maxAttachmentBytes":   handlers.MaxAttachmentBytes,
		})
	}))

	// TODO: register remaining handlers — groups, activity, events (SSE).

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
