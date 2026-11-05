package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"

	"github.com/obot-platform/obot-sentry/pkg/eventlog"
	"github.com/obot-platform/obot-sentry/pkg/state"
)

// A server rejection exits 3, while errors that already carry an exit code keep it.
func TestRejectedExitCode(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantExitCode int
	}{
		{
			name:         "nil",
			err:          nil,
			wantExitCode: 0,
		},
		{
			name:         "rejected",
			err:          types.NewErrHTTP(http.StatusUnauthorized, "unauthorized"),
			wantExitCode: exitCodeRejected,
		},
		{
			name:         "configuration error keeps its code",
			err:          NewConfigError(errors.New("no server URL configured")),
			wantExitCode: exitCodeConfig,
		},
		{
			name:         "other failure",
			err:          errors.New("connection refused"),
			wantExitCode: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectedExitCode(tt.err)
			gotExitCode := 0
			if err != nil {
				gotExitCode = 1
				if exitErr, ok := errors.AsType[*ExitCodeError](err); ok {
					gotExitCode = exitErr.Code
				}
			}
			if gotExitCode != tt.wantExitCode {
				t.Fatalf("exit code = %d, want %d", gotExitCode, tt.wantExitCode)
			}
		})
	}
}

// Scheduled scans log a failure when it first happens or changes, and the
// recovery after it, but not the same failure on every run.
func TestScanEventID(t *testing.T) {
	failure := errors.New("enrolling device: rejected")
	tests := []struct {
		name     string
		previous state.ScanState
		err      error
		want     uint32
	}{
		{
			name:     "first failure",
			previous: state.ScanState{LastStatus: "ok"},
			err:      failure,
			want:     eventlog.ScanFailed,
		},
		{
			name:     "first scan fails",
			previous: state.ScanState{},
			err:      failure,
			want:     eventlog.ScanFailed,
		},
		{
			name:     "same failure again",
			previous: state.ScanState{LastStatus: "error", LastError: failure.Error(), ReportedError: failure.Error()},
			err:      failure,
			want:     0,
		},
		{
			name:     "different failure",
			previous: state.ScanState{LastStatus: "error", LastError: "dial tcp: connection refused", ReportedError: "dial tcp: connection refused"},
			err:      failure,
			want:     eventlog.ScanFailed,
		},
		{
			// A version without event logging recorded the failure but never reported it.
			name:     "failure recorded before an upgrade",
			previous: state.ScanState{LastStatus: "error", LastError: failure.Error()},
			err:      failure,
			want:     eventlog.ScanFailed,
		},
		{
			name:     "recovery",
			previous: state.ScanState{LastStatus: "error", LastError: failure.Error(), ReportedError: failure.Error()},
			err:      nil,
			want:     eventlog.ScanRecovered,
		},
		{
			// Nothing was reported, so there's nothing to recover from.
			name:     "recovery from a failure recorded before an upgrade",
			previous: state.ScanState{LastStatus: "error", LastError: failure.Error()},
			err:      nil,
			want:     0,
		},
		{
			name:     "still healthy",
			previous: state.ScanState{LastStatus: "ok"},
			err:      nil,
			want:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scanEventID(tt.previous, tt.err); got != tt.want {
				t.Fatalf("scanEventID() = %d, want %d", got, tt.want)
			}
		})
	}
}
