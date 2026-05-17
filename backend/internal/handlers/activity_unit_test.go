package handlers

import (
	"strconv"
	"strings"
	"testing"

	"trove/backend/internal/data"
)

func sp(s string) *string { return &s }

func TestItemDiff(t *testing.T) {
	base := &data.Item{
		Title: "A", Kind: data.ItemKindTask, Status: data.ItemStatusOpen,
		Position: 1, Body: sp("hello\nworld"),
	}

	t.Run("changed fields only; body untouched", func(t *testing.T) {
		after := *base
		after.Title = "B"
		after.Status = data.ItemStatusInProgress
		d := itemDiff(base, &after)
		if _, ok := d["title"]; !ok {
			t.Error("missing title")
		}
		if _, ok := d["status"]; !ok {
			t.Error("missing status")
		}
		if _, ok := d["body"]; ok {
			t.Error("body present though unchanged")
		}
		if _, ok := d["position"]; ok {
			t.Error("position present though unchanged")
		}
	})

	t.Run("body change → patch capturing the delta", func(t *testing.T) {
		after := *base
		after.Body = sp("hello\nthere")
		b, ok := itemDiff(base, &after)["body"].(map[string]any)
		if !ok {
			t.Fatal("body diff not a map")
		}
		ds, ok := b["diff"].(string)
		if !ok || ds == "" {
			t.Fatalf("body.diff missing/empty: %v", b)
		}
		// Format-agnostic: the patch must reference both changed tokens.
		if !strings.Contains(ds, "there") || !strings.Contains(ds, "world") {
			t.Errorf("diff doesn't reference the change:\n%s", ds)
		}
	})

	t.Run("identical → empty", func(t *testing.T) {
		cp := *base
		if d := itemDiff(base, &cp); len(d) != 0 {
			t.Errorf("want empty, got %v", d)
		}
	})
}

func TestTextChange_TruncatesPathologicalRewrite(t *testing.T) {
	old := sp("x")
	var sb strings.Builder
	for i := 0; i < 5000; i++ {
		sb.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	huge := sb.String()

	r := textChange(old, &huge)
	if r["truncated"] != true {
		t.Fatalf("want truncated=true, got %v", r)
	}
	if _, ok := r["diff"]; ok {
		t.Error("truncated result must not also carry a diff")
	}
	if r["new_lines"] == nil {
		t.Error("truncated result should summarise new_lines")
	}
}

func TestProjectDiff(t *testing.T) {
	before := &data.Project{Name: "Garden", Slug: "garden", Description: sp("old desc")}
	after := &data.Project{Name: "Garden Plans", Slug: "garden", Description: sp("new desc")}

	d := projectDiff(before, after)
	if _, ok := d["name"]; !ok {
		t.Error("missing name")
	}
	if _, ok := d["slug"]; ok {
		t.Error("slug present though unchanged")
	}
	desc, ok := d["description"].(map[string]any)
	if !ok || desc["diff"] == nil {
		t.Errorf("description diff missing: %v", d["description"])
	}
}
