# Trove

> A cozy little place to keep your projects, ideas, and the things you're chasing — built to play nicely with Claude and other AI tools.

## Stack

- **Frontend:** SvelteKit (Svelte 5), Tailwind, served as a static SPA
- **Backend:** Go, Goose migrations
- **Database:** PostgreSQL (with `tsvector` full-text search)
- **Object storage (coming soon):** Tigris (prod) / MinIO (dev)
- **Auth:** Supabase (auth-only — app data lives in our own Postgres)
- **Real-time (coming soon):** Server-Sent Events, with Postgres `LISTEN`/`NOTIFY` fan-out
- **Hosting:** Fly.io · **Local dev:** Tilt · **Frontend package manager:** bun

## Run locally

```bash
cp backend/.env.example backend/.env
# edit backend/.env: set SUPABASE_URL=https://<your-project>.supabase.co

tilt up
```

Ports: frontend `:3003`, backend `:8082`, Postgres `:5434`, MinIO `:9000` (S3 API) / `:9001` (web console).
