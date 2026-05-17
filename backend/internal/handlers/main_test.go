package handlers

import (
	"os"
	"testing"

	"trove/backend/internal/data/datatest"
)

// Wires the shared-container lifecycle for this test binary.
func TestMain(m *testing.M) { os.Exit(datatest.Main(m)) }
