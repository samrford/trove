package jobs

import (
	"sort"
	"testing"
	"time"

	"trove/backend/internal/data/storage"
)

func TestOrphans_HappyPath(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-OrphanGracePeriod)
	objects := []storage.ObjectInfo{
		// known + old → keep (referenced)
		{Key: "projects/p/items/i/known-old.jpg", LastModified: cutoff.Add(-1 * time.Hour)},
		// unknown + old → purge
		{Key: "projects/p/items/i/orphan-old.jpg", LastModified: cutoff.Add(-1 * time.Hour)},
		// unknown + recent → keep (within grace window)
		{Key: "projects/p/items/i/orphan-fresh.jpg", LastModified: now.Add(-5 * time.Minute)},
		// known + recent → keep
		{Key: "projects/p/items/i/known-fresh.jpg", LastModified: now.Add(-5 * time.Minute)},
	}
	known := map[string]struct{}{
		"projects/p/items/i/known-old.jpg":   {},
		"projects/p/items/i/known-fresh.jpg": {},
	}

	got := orphans(objects, known, now)
	sort.Strings(got)
	want := []string{"projects/p/items/i/orphan-old.jpg"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("orphans = %v, want %v", got, want)
	}
}

func TestOrphans_GraceWindowBoundary(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	// Object modified exactly at the cutoff is considered "not after the
	// cutoff" — i.e. eligible for purge. Anything strictly after is safe.
	cutoff := now.Add(-OrphanGracePeriod)
	objects := []storage.ObjectInfo{
		{Key: "at-cutoff", LastModified: cutoff},                 // purge
		{Key: "after-cutoff", LastModified: cutoff.Add(1 * time.Second)}, // keep
	}
	got := orphans(objects, map[string]struct{}{}, now)
	if len(got) != 1 || got[0] != "at-cutoff" {
		t.Fatalf("expected only 'at-cutoff', got %v", got)
	}
}

func TestOrphans_AllKnown(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	objects := []storage.ObjectInfo{
		{Key: "a", LastModified: now.Add(-24 * time.Hour)},
		{Key: "b", LastModified: now.Add(-48 * time.Hour)},
	}
	known := map[string]struct{}{"a": {}, "b": {}}
	if got := orphans(objects, known, now); len(got) != 0 {
		t.Fatalf("expected no orphans, got %v", got)
	}
}

func TestOrphans_Empty(t *testing.T) {
	if got := orphans(nil, nil, time.Now()); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
