package data_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"trove/backend/internal/data"
	"trove/backend/internal/data/datatest"
)

func TestFindOrCreateTag_Create(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)

	var tag *data.Tag
	var created bool
	txDo(t, db, func(tx *sql.Tx) error {
		var e error
		tag, created, e = data.FindOrCreateTag(context.Background(), tx, user, "Auth", "", nil, nil, nil)
		return e
	})
	if !created || tag == nil || tag.Name != "Auth" {
		t.Fatalf("created=%v tag=%+v, want created=true name=Auth", created, tag)
	}
}

func TestFindOrCreateTag_AutoMergeByName(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)

	// Distinct explicit slugs so only tags_owner_normalised_unique collides —
	// isolates the auto-merge-after-23505 path the SAVEPOINT exists to protect.
	var firstID, secondID string
	var secondCreated bool
	txDo(t, db, func(tx *sql.Tx) error {
		t1, _, err := data.FindOrCreateTag(context.Background(), tx, user, "Infra", "infra-one", nil, nil, nil)
		if err != nil {
			return err
		}
		firstID = t1.ID
		t2, created, err := data.FindOrCreateTag(context.Background(), tx, user, "infra", "infra-two", nil, nil, nil)
		if err != nil {
			return err
		}
		secondID, secondCreated = t2.ID, created
		return nil
	})
	if secondCreated || secondID != firstID {
		t.Fatalf("created=%v id=%s, want created=false id=%s (auto-merge)", secondCreated, secondID, firstID)
	}
}

func TestFindOrCreateTag_SlugCollisionErrors(t *testing.T) {
	db := datatest.OpenTestDB(t)
	user := datatest.SeedUser(t, db)

	// First tag claims slug "dup". Second, a *different* name, forces the same
	// slug → tags_owner_slug_unique, which must surface as ErrTagSlugTaken
	// (and the SAVEPOINT must keep the tx alive to return it cleanly).
	err := data.WithRetry(context.Background(), db, func(tx *sql.Tx) error {
		if _, _, e := data.FindOrCreateTag(context.Background(), tx, user, "First", "dup", nil, nil, nil); e != nil {
			return e
		}
		_, _, e := data.FindOrCreateTag(context.Background(), tx, user, "Second", "dup", nil, nil, nil)
		return e
	})
	if !errors.Is(err, data.ErrTagSlugTaken) {
		t.Fatalf("err = %v, want ErrTagSlugTaken", err)
	}
}
