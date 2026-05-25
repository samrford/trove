package events_test

import (
	"os"
	"testing"

	"trove/backend/internal/data/datatest"
)

// Wires the shared-container lifecycle for this test binary (see package
// datatest docs — every DB-backed test package needs exactly this).
func TestMain(m *testing.M) { os.Exit(datatest.Main(m)) }
