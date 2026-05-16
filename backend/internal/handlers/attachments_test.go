package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"trove/backend/internal/data"
	"trove/backend/internal/data/storage/storagetest"
)

func TestBuildStorageKey_PreservesExtension(t *testing.T) {
	key := buildStorageKey("proj-1", "item-9", "vacation photo.heic")
	if !strings.HasPrefix(key, "projects/proj-1/items/item-9/") {
		t.Errorf("prefix wrong: %q", key)
	}
	if !strings.HasSuffix(key, ".heic") {
		t.Errorf("extension missing: %q", key)
	}
}

func TestBuildStorageKey_NoExtension(t *testing.T) {
	key := buildStorageKey("p", "i", "Makefile")
	if strings.HasSuffix(key, ".") || strings.Count(key, ".") > 0 {
		t.Errorf("expected no trailing dot/ext, got %q", key)
	}
}

func TestBuildStorageKey_UniquePerCall(t *testing.T) {
	a := buildStorageKey("p", "i", "x.png")
	b := buildStorageKey("p", "i", "x.png")
	if a == b {
		t.Fatalf("expected unique keys, both = %s", a)
	}
}

func TestSignAttachments_PopulatesURLs(t *testing.T) {
	store := storagetest.NewFake()
	store.Put("a.jpg", []byte("a"), "image/jpeg", time.Now())
	store.Put("b.png", []byte("b"), "image/png", time.Now())

	atts := []data.Attachment{
		{ID: "1", StorageKey: "a.jpg", Filename: "a.jpg"},
		{ID: "2", StorageKey: "b.png", Filename: "b.png"},
	}
	got, err := SignAttachments(context.Background(), store, atts)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(got))
	}
	if !strings.Contains(got[0].URL, "a.jpg") {
		t.Errorf("first URL missing key: %q", got[0].URL)
	}
	if !strings.Contains(got[1].URL, "b.png") {
		t.Errorf("second URL missing key: %q", got[1].URL)
	}
}

func TestSignAttachments_EmptySlice(t *testing.T) {
	got, err := SignAttachments(context.Background(), storagetest.NewFake(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d", len(got))
	}
}
