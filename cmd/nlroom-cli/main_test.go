package main

import (
	"errors"
	"github.com/nodelane/nodelane-room/internal/model"
	"testing"
)

func TestFiniteExitCodes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{errors.New("invalid flag"), 2}, {model.Failure("request_state_stale"), 2}, {model.Failure("local_service_unavailable"), 3}, {model.Failure("system_internal_error"), 3}, {model.Failure("local_rpc_timeout"), 4}, {model.Failure("operation_pending"), 4}, {model.Failure("operation_expired"), 4}} {
		if got := exitCode(tc.err); got != tc.want {
			t.Errorf("%v: got %d want %d", tc.err, got, tc.want)
		}
	}
}
