package data_test

import (
	"os"
	"testing"

	"trove/backend/internal/data/datatest"
)

// TestMain wires the shared-container lifecycle. Every DB-backed test package
// needs exactly this one-liner (see package datatest docs).
func TestMain(m *testing.M) { os.Exit(datatest.Main(m)) }
