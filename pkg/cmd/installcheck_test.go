package cmd

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

// An installer's key check fails only when the server rejects the key; anything
// else, including a missing configuration, lets the install finish.
func TestInstallCheckResult(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantExitCode int
		wantStdout   string
		wantStderr   string
	}{
		{
			name:         "accepted",
			err:          nil,
			wantExitCode: 0,
			wantStdout:   "enrollment key was accepted by https://obot.example.com",
		},
		{
			name:         "rejected key fails the install",
			err:          types.NewErrHTTP(http.StatusUnauthorized, "unauthorized"),
			wantExitCode: exitCodeRejected,
		},
		{
			name:         "server without the check lets the install finish",
			err:          types.NewErrHTTP(http.StatusForbidden, "forbidden"),
			wantExitCode: 0,
			wantStderr:   "the scheduled scan will enroll the device later",
		},
		{
			name:         "unknown route lets the install finish",
			err:          types.NewErrHTTP(http.StatusNotFound, "not found"),
			wantExitCode: 0,
			wantStderr:   "the scheduled scan will enroll the device later",
		},
		{
			name:         "unreachable server lets the install finish",
			err:          errors.New("dial tcp: connection refused"),
			wantExitCode: 0,
			wantStderr:   "connection refused",
		},
		{
			name:         "server error lets the install finish",
			err:          types.NewErrHTTP(http.StatusServiceUnavailable, "unavailable"),
			wantExitCode: 0,
			wantStderr:   "the scheduled scan will enroll the device later",
		},
		{
			name:         "missing configuration lets the install finish",
			err:          NewConfigError(errors.New("no enrollment key configured")),
			wantExitCode: 0,
			wantStderr:   "no enrollment key configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)

			err := installCheckResult(cmd, "https://obot.example.com", tt.err)

			gotExitCode := 0
			if err != nil {
				exitErr, ok := errors.AsType[*ExitCodeError](err)
				if !ok {
					t.Fatalf("expected an *ExitCodeError, got %T: %v", err, err)
				}
				gotExitCode = exitErr.Code
			}
			if gotExitCode != tt.wantExitCode {
				t.Fatalf("exit code = %d, want %d (err %v)", gotExitCode, tt.wantExitCode, err)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Fatalf("stdout %q does not contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr %q does not contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
