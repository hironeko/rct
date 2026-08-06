package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hironeko/rct/internal/domain"
	"github.com/hironeko/rct/internal/store/filesystem"
)

type liveOptions struct {
	Progress string
	Notify   string
	Writer   bool
}

type liveObserver struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func (o *liveObserver) Stop() {
	if o == nil {
		return
	}
	o.once.Do(func() { close(o.stop); <-o.done })
}

func validateLiveOptions(progress, notify string) error {
	if !oneOf(progress, "auto", "tty", "plain", "jsonl", "none") {
		return fmt.Errorf("--progress must be auto, tty, plain, jsonl, or none")
	}
	if !oneOf(notify, "auto", "desktop", "bell", "none") {
		return fmt.Errorf("--notify must be auto, desktop, bell, or none")
	}
	return nil
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func (c *CLI) observeRun(ctx context.Context, store *filesystem.Store, runID string, options liveOptions) *liveObserver {
	progressMode := resolveProgressMode(options.Progress, c.stderr, os.Getenv)
	notifyMode := resolveNotifyMode(options.Notify, c.stderr, options.Writer, os.Getenv)
	if progressMode == "none" && notifyMode == "none" {
		return nil
	}
	observer := &liveObserver{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(observer.done)
		c.runObserver(ctx, store, runID, progressMode, notifyMode, observer.stop, true, false)
	}()
	return observer
}

func resolveProgressMode(mode string, writer io.Writer, getenv func(string) string) string {
	if mode != "auto" {
		return mode
	}
	if isInteractive(writer) && getenv("CI") == "" && getenv("TERM") != "dumb" {
		return "tty"
	}
	return "none"
}

func resolveNotifyMode(mode string, writer io.Writer, longWriter bool, getenv func(string) string) string {
	if mode != "auto" {
		return mode
	}
	if longWriter && isInteractive(writer) && getenv("CI") == "" {
		return "auto"
	}
	return "none"
}

func isInteractive(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (c *CLI) runObserver(
	ctx context.Context,
	store *filesystem.Store,
	runID, progressMode, notifyMode string,
	stop <-chan struct{},
	suppressReplayNotifications bool,
	stopAtTerminal bool,
) {
	notifier := newLocalNotifier(notifyMode, c.stderr)
	var lastSeq, lastActivityRevision uint64
	var lastLine string
	initial := true
	render := func(final bool) bool {
		snapshot, err := store.Progress(runID)
		if err != nil {
			if !initial {
				fmt.Fprintf(c.stderr, "rct progress: %v\n", err)
			}
			return false
		}
		if initial {
			lastSeq = snapshot.LastEventSeq
			lastActivityRevision = activityRevision(snapshot.Activity)
			if progressMode != "none" {
				lastLine = renderSnapshot(c.stderr, progressMode, snapshot, "", final)
			}
			initial = false
			return terminalState(snapshot.State)
		}
		events, eventErr := store.ReadEvents(runID, lastSeq)
		if eventErr == nil {
			for _, event := range events {
				if progressMode == "plain" {
					renderPlainEvent(c.stderr, event)
				}
				if progressMode == "jsonl" {
					renderJSONLine(c.stderr, map[string]any{"kind": "event", "event": event})
				}
				if !suppressReplayNotifications || event.Sequence > lastSeq {
					notifier.Notify(event, runID)
				}
				lastSeq = event.Sequence
			}
		}
		activityChanged := activityRevision(snapshot.Activity) != lastActivityRevision
		if progressMode == "tty" && (activityChanged || final || len(events) > 0) {
			lastLine = renderSnapshot(c.stderr, progressMode, snapshot, lastLine, final)
		} else if progressMode == "plain" && activityChanged && len(events) == 0 {
			renderPlainSnapshot(c.stderr, snapshot)
		} else if progressMode == "jsonl" && activityChanged && len(events) == 0 {
			renderJSONLine(c.stderr, map[string]any{"kind": "snapshot", "snapshot": snapshot})
		}
		lastActivityRevision = activityRevision(snapshot.Activity)
		return terminalState(snapshot.State)
	}

	if render(false) && stopAtTerminal {
		return
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			render(true)
			return
		case <-stop:
			render(true)
			return
		case <-ticker.C:
			if render(false) && stopAtTerminal {
				return
			}
		}
	}
}

func activityRevision(activity *domain.CurrentActivity) uint64 {
	if activity == nil {
		return 0
	}
	return activity.Revision
}

func terminalState(state domain.WorkflowState) bool {
	switch state {
	case domain.StateCompleted, domain.StateFailed, domain.StateBlocked, domain.StateWaitingForHuman, domain.StateAwaitingApproval:
		return true
	default:
		return false
	}
}

func renderSnapshot(writer io.Writer, mode string, snapshot domain.ProgressSnapshot, previous string, final bool) string {
	if mode == "plain" {
		renderPlainSnapshot(writer, snapshot)
		return ""
	}
	if mode == "jsonl" {
		renderJSONLine(writer, map[string]any{"kind": "snapshot", "snapshot": snapshot})
		return ""
	}
	if mode != "tty" {
		return ""
	}
	line := snapshotLine(snapshot)
	if line == previous && !final {
		return previous
	}
	fmt.Fprintf(writer, "\r\x1b[2K%s", line)
	if final {
		fmt.Fprintln(writer)
	}
	return line
}

func snapshotLine(snapshot domain.ProgressSnapshot) string {
	gauge := domain.Gauge{Total: 1, Label: "phases complete"}
	if len(snapshot.Gauges) > 0 {
		gauge = snapshot.Gauges[0]
	}
	width := 16
	filled := 0
	if gauge.Total > 0 {
		filled = gauge.Completed * width / gauge.Total
	}
	bar := "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
	activity := string(snapshot.State)
	if snapshot.Activity != nil {
		parts := []string{phaseDisplay(snapshot.Activity.Phase)}
		if snapshot.Activity.Provider != "" {
			parts = append(parts, strings.Title(snapshot.Activity.Provider)+" "+snapshot.Activity.Action)
		} //nolint:staticcheck
		if snapshot.Activity.Round > 0 {
			parts = append(parts, fmt.Sprintf("round %d/%d", snapshot.Activity.Round, snapshot.Activity.MaxRounds))
		}
		parts = append(parts, activityLiveness(snapshot.Activity))
		activity = strings.Join(parts, " · ")
	}
	return fmt.Sprintf("Overall %s %d/%d %s | %s", bar, gauge.Completed, gauge.Total, gauge.Label, activity)
}

func renderPlainSnapshot(writer io.Writer, snapshot domain.ProgressSnapshot) {
	gauge := snapshot.Gauges[0]
	fmt.Fprintf(writer, "%s progress state=%s phases=%d/%d", time.Now().UTC().Format(time.RFC3339), snapshot.State, gauge.Completed, gauge.Total)
	if snapshot.Activity != nil {
		fmt.Fprintf(writer, " phase=%s action=%s job=%s", snapshot.Activity.Phase, snapshot.Activity.Action, snapshot.Activity.JobID)
	}
	fmt.Fprintln(writer)
}

func renderPlainEvent(writer io.Writer, event domain.ProgressEvent) {
	fmt.Fprintf(writer, "%s %s", event.Timestamp.UTC().Format(time.RFC3339), snakeEventName(event.Type))
	if event.Phase != "" {
		fmt.Fprintf(writer, " phase=%s", event.Phase)
	}
	if event.Role != "" {
		fmt.Fprintf(writer, " role=%s", event.Role)
	}
	if event.Provider != "" {
		fmt.Fprintf(writer, " provider=%s", event.Provider)
	}
	if event.Round > 0 {
		fmt.Fprintf(writer, " round=%d", event.Round)
	}
	if event.JobID != "" {
		fmt.Fprintf(writer, " job=%s", event.JobID)
	}
	fmt.Fprintln(writer)
}

func snakeEventName(value string) string {
	var result strings.Builder
	for index, char := range value {
		if index > 0 && char >= 'A' && char <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(char)
	}
	return strings.ToLower(result.String())
}

func renderJSONLine(writer io.Writer, value any) { _ = json.NewEncoder(writer).Encode(value) }

func phaseDisplay(phase string) string {
	return strings.Title(strings.ReplaceAll(phase, "_", " ")) //nolint:staticcheck
}

func activityLiveness(activity *domain.CurrentActivity) string {
	if activity.Status == domain.ActivityStale {
		return "stale"
	}
	if activity.Status != domain.ActivityRunning {
		return string(activity.Status)
	}
	seconds := int(time.Since(activity.LastHeartbeatAt).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("last activity %ds ago", seconds)
}

type localNotifier struct {
	mode   string
	writer io.Writer
	seen   map[string]bool
	warned bool
}

func newLocalNotifier(mode string, writer io.Writer) *localNotifier {
	return &localNotifier{mode: mode, writer: writer, seen: map[string]bool{}}
}

func (n *localNotifier) Notify(event domain.ProgressEvent, runID string) {
	if n.mode == "none" {
		return
	}
	title, body, ok := notificationText(event)
	if !ok {
		return
	}
	key := fmt.Sprintf("%s:%d:%s", runID, event.Sequence, n.mode)
	if n.seen[key] {
		return
	}
	n.seen[key] = true
	if short := safeShortRunID(runID); short != "" {
		body += " (" + short + ")"
	}
	if n.mode == "bell" {
		fmt.Fprint(n.writer, "\a")
		return
	}
	if err := sendDesktopNotification(title, body); err == nil {
		return
	}
	if n.mode == "auto" {
		fmt.Fprint(n.writer, "\a")
		return
	}
	if !n.warned {
		fmt.Fprintln(n.writer, "rct: desktop notifications are unavailable; workflow continues")
		n.warned = true
	}
}

func notificationText(event domain.ProgressEvent) (string, string, bool) {
	kind := event.Type
	switch {
	case kind == "HumanActionRequired" || kind == "ImplementationApprovalRequested":
		return "rct: approval required", "Review the pending approval", true
	case kind == "RunCompleted":
		return "rct: run completed", "The run finished successfully", true
	case kind == "RunFailed":
		return "rct: run failed", "Open rct status for the next action", true
	case kind == "RetryLimitReached" || strings.Contains(kind, "LimitReached"):
		return "rct: review required", "The retry limit was reached", true
	case kind == "ActivityStale":
		return "rct: activity stale", "Check the current process or backend", true
	default:
		return "", "", false
	}
}

func safeShortRunID(runID string) string {
	parts := strings.Split(runID, "_")
	if len(parts) != 3 || parts[0] != "run" || len(parts[2]) != 12 {
		return ""
	}
	for _, char := range parts[2] {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return ""
		}
	}
	return parts[2][:8]
}

func sendDesktopNotification(title, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		script := `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`
		return exec.CommandContext(ctx, "osascript", "-e", script, title, body).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return err
		}
		return exec.CommandContext(ctx, "notify-send", title, body).Run()
	default:
		return errors.New("desktop notifications are unsupported")
	}
}
