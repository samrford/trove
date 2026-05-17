package data_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/lib/pq"

	"trove/backend/internal/data"
	"trove/backend/internal/data/datatest"
)

func TestWithRetry_Policy(t *testing.T) {
	db := datatest.OpenTestDB(t)

	cases := []struct {
		name      string
		failWith  error // returned by fn until failTimes is exhausted
		failTimes int   // how many attempts return failWith before nil
		wantCalls int   // expected fn invocations
		wantErr   bool  // WithRetry returns non-nil
	}{
		{
			name:      "deadlock retries then succeeds",
			failWith:  &pq.Error{Code: "40P01"}, // deadlock_detected
			failTimes: 2,
			wantCalls: 3, // 2 failures + 1 success, within the 3-attempt cap
			wantErr:   false,
		},
		{
			name:      "serialization failure exhausts attempts",
			failWith:  &pq.Error{Code: "40001"}, // serialization_failure
			failTimes: 99,
			wantCalls: 3, // capped at maxTxAttempts
			wantErr:   true,
		},
		{
			name:      "unique violation is not retried",
			failWith:  &pq.Error{Code: "23505"}, // unique_violation — logic error
			failTimes: 99,
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:      "plain error is not retried",
			failWith:  errors.New("boom"),
			failTimes: 99,
			wantCalls: 1,
			wantErr:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := data.WithRetry(context.Background(), db, func(tx *sql.Tx) error {
				calls++
				if calls <= tc.failTimes {
					return tc.failWith
				}
				return nil
			})
			if calls != tc.wantCalls {
				t.Errorf("fn called %d times, want %d", calls, tc.wantCalls)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestWithRetry_CancelledContextDoesNotRun(t *testing.T) {
	db := datatest.OpenTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the first attempt

	calls := 0
	err := data.WithRetry(ctx, db, func(tx *sql.Tx) error {
		calls++
		return nil
	})
	if calls != 0 {
		t.Errorf("fn called %d times, want 0 (cancelled ctx must not run the unit)", calls)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
