package provisioner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/plan"
)

const (
	restartCommand      = `shutdown.exe /r /f /t 5 /c "packer-plugin-windows-optimize: restart after applying optimizations"`
	probeInterval       = 10 * time.Second
	probeAttemptTimeout = 2 * time.Minute
)

// restartAndReverify restarts the guest, waits for the boot time to change,
// reinstalls the scripts, checks WinRM, re-inspects changes and passes the
// states to reverify.
//
// Parameters:
//   - changes: The verified changes whose targets are re-inspected.
//   - reverify: Marks changes that no longer hold their value as failed.
//
// Returns:
//   - error: The restart failed, the guest did not come back within
//     restart_timeout, WinRM regressed, or re-inspection failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) restartAndReverify(ctx context.Context, changes []*plan.ChangeItem, reverify func(plan.States, string)) error {
	before, err := r.probe(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the boot time before restarting: %w", err)
	}
	r.ui.Say("Restarting the machine...")
	if err := r.remote.exec(ctx, restartCommand); err != nil {
		return fmt.Errorf("restart command failed: %w", err)
	}
	r.ui.Say(fmt.Sprintf("Waiting up to %s for the machine to restart...", r.cfg.RestartTimeout))
	if _, err := r.probeUntil(ctx, r.now().Add(r.cfg.RestartTimeout), before.BootTime); err != nil {
		r.addCheck("WinRM after restart", false, err.Error())
		return fmt.Errorf("the machine did not come back over WinRM within %s: %w", r.cfg.RestartTimeout, err)
	}
	r.reboot.Performed = true
	if err := r.remote.install(ctx); err != nil {
		return err
	}
	if err := r.checkConnectivity(ctx, "after restart"); err != nil {
		return err
	}
	current, err := r.inspect(ctx, changes)
	if err != nil {
		return err
	}
	reverify(current, "after restart")
	return nil
}

// probeUntil repeats the Probe action every probeInterval until it succeeds or
// deadline passes. When previousBootTime is set, a probe only counts once the
// boot time has changed, which is how a restart is detected. After a failed
// attempt the guest scripts are uploaded again before the next one, in case
// the restart removed the working directory.
//
// Returns:
//   - *facts.Probe: The first successful probe.
//   - error: The last failure when the deadline passes, or the context error.
//
// Author: Lars Krogh
// Version: 1.1.0
func (r *run) probeUntil(ctx context.Context, deadline time.Time, previousBootTime string) (*facts.Probe, error) {
	var lastErr error
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, probeAttemptTimeout)
		var p *facts.Probe
		var err error
		if lastErr != nil {
			err = r.remote.install(attemptCtx)
		}
		if err == nil {
			p, err = r.probe(attemptCtx)
		}
		cancel()
		switch {
		case err == nil && (previousBootTime == "" || p.BootTime != previousBootTime):
			return p, nil
		case err == nil:
			lastErr = errors.New("the machine has not restarted yet")
		default:
			lastErr = err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !r.now().Add(probeInterval).Before(deadline) {
			return nil, lastErr
		}
		if err := sleepUntil(ctx, r.now().Add(probeInterval), r.now); err != nil {
			return nil, err
		}
	}
}

// sleepUntil waits until time t as measured by now, or until ctx is done.
//
// Returns:
//   - error: The context error, or nil.
//
// Author: Lars Krogh
// Version: 1.0.0
func sleepUntil(ctx context.Context, t time.Time, now func() time.Time) error {
	d := t.Sub(now())
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
