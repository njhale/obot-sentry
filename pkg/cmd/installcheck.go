package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"

	"github.com/obot-platform/obot-sentry/pkg/client"
	"github.com/obot-platform/obot-sentry/pkg/eventlog"
)

// InstallCheck is the Windows installer's check that the server accepts the configured enrollment key.
type InstallCheck struct {
	ConfigFlags
	Timeout int `usage:"Number of seconds to wait for the server" default:"60" env:"OBOT_SENTRY_INSTALL_CHECK_TIMEOUT"`
}

func (c *InstallCheck) Customize(cmd *cobra.Command) {
	cmd.Use = "install-check"
	cmd.Short = "Check the configured enrollment key with the server, for installers"
	cmd.Long = "Install-check asks the Obot server whether it accepts the configured enrollment key, " +
		"without enrolling this device, and writes the result to the event log. It exits 3 only " +
		"when the server rejects the key. Any other failure, such as an unreachable server, exits " +
		"0 so the installer finishes and scheduled scans enroll the device later."
	cmd.Hidden = true
	cmd.Args = cobra.NoArgs
}

// Run checks the enrollment key and reports the outcome to the event log and the exit code, since nobody watches an
// installer's output.
func (c *InstallCheck) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Timeout)*time.Second)
		defer cancel()
	}

	serverURL, err := c.check(ctx)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil {
		// Our timeout fired. main exits silently on context errors, so replace it with one that says what happened.
		err = fmt.Errorf("no response from %s within %d seconds", serverURL, c.Timeout)
	}
	return installCheckResult(cmd, serverURL, err)
}

// check resolves the deployment configuration and asks the configured server whether it accepts the enrollment key.
// It returns the server URL alongside the result, even on failure, so the caller can name the server in its report.
func (c *InstallCheck) check(ctx context.Context) (string, error) {
	cfg, err := c.resolve()
	if err != nil {
		return "", NewConfigError(err)
	}
	if cfg.ServerURL == "" {
		return "", NewConfigError(fmt.Errorf("no server URL configured (flag, env, or MDM)"))
	}
	if cfg.EnrollmentKey == "" {
		return cfg.ServerURL, NewConfigError(fmt.Errorf("no enrollment key configured (flag, env, or MDM)"))
	}
	return cfg.ServerURL, client.New(cfg.ServerURL).CheckEnrollmentCredential(ctx, cfg.EnrollmentKey)
}

// installCheckResult logs an installer's key check to the event log and decides whether the install continues.
// Only a rejected key stops the install (exit 3): rolling back then leaves nothing behind, instead of a device that
// can never enroll and, with enforcement on, blocks every tool call.
// Any other failure lets the install finish. An unreachable server or a server error is expected while a device is
// still being provisioned, and a server too old to check keys answers a valid one with a 403; scheduled scans enroll
// the device once the server answers.
func installCheckResult(cmd *cobra.Command, serverURL string, err error) error {
	switch {
	case err == nil:
		msg := fmt.Sprintf("Obot Sentry's enrollment key was accepted by %s.", serverURL)
		eventlog.Info(eventlog.KeyAccepted, msg)
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), msg)
		return nil
	case isKeyRejected(err):
		err = fmt.Errorf("the enrollment key was rejected because it is invalid, expired, or deleted: %w", err)
		// The MSI shows only a generic error for a failed custom action, so this event carries the reason.
		eventlog.Error(eventlog.KeyRejected, fmt.Sprintf("Obot Sentry installation stopped because %s rejected its enrollment key: %v", serverURL, err))
		return &ExitCodeError{Code: exitCodeRejected, Err: err}
	default:
		msg := fmt.Sprintf("Obot Sentry could not check its enrollment key with %s during installation; the scheduled scan will enroll the device later: %v", serverURL, err)
		eventlog.Warning(eventlog.KeyCheckDeferred, msg)
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+msg)
		return nil
	}
}

// isKeyRejected reports whether the server refused the enrollment key itself.
// Every server version answers a bad key with a 401, while other 4xx responses can come from a server that predates
// the check or from a proxy in the way, so they don't prove anything about the key.
func isKeyRejected(err error) bool {
	httpErr, ok := errors.AsType[*types.ErrHTTP](err)
	return ok && httpErr.Code == http.StatusUnauthorized
}
