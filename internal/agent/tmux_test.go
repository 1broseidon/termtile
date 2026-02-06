//go:build !windows

package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupStubTmux(t *testing.T) (stubDir string, logPath string) {
	t.Helper()

	dir := t.TempDir()
	tmuxPath := filepath.Join(dir, "tmux")
	logPath = filepath.Join(dir, "tmux.log")

	script := `#!/bin/sh
set -eu

if [ -n "${TMUX_STUB_LOG:-}" ]; then
  printf '%s\n' "$*" >> "${TMUX_STUB_LOG}"
fi

cmd="${1:-}"
case "$cmd" in
  has-session)
    if [ -n "${TMUX_STUB_HAS_SESSION_EXIT:-}" ]; then
      exit "${TMUX_STUB_HAS_SESSION_EXIT}"
    fi
    session=""
    shift
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-t" ]; then
        shift
        session="${1:-}"
        break
      fi
      shift
    done
    if [ -n "${TMUX_STUB_MISSING_SESSION:-}" ] && [ "$session" = "${TMUX_STUB_MISSING_SESSION}" ]; then
      exit 1
    fi
    exit 0
    ;;
  send-keys)
    last=""
    for arg in "$@"; do
      last="$arg"
    done
    if [ -n "${TMUX_STUB_SEND_KEYS_FAIL_ON:-}" ] && [ "$last" = "${TMUX_STUB_SEND_KEYS_FAIL_ON}" ]; then
      if [ -n "${TMUX_STUB_SEND_KEYS_STDERR:-}" ]; then
        printf '%s\n' "${TMUX_STUB_SEND_KEYS_STDERR}" 1>&2
      fi
      exit 1
    fi
    exit 0
    ;;
  capture-pane)
    if [ -n "${TMUX_STUB_CAPTURE_PANE_EXIT:-}" ]; then
      if [ -n "${TMUX_STUB_CAPTURE_PANE_STDERR:-}" ]; then
        printf '%s\n' "${TMUX_STUB_CAPTURE_PANE_STDERR}" 1>&2
      fi
      if [ -n "${TMUX_STUB_CAPTURE_PANE_OUTPUT:-}" ]; then
        printf '%s' "${TMUX_STUB_CAPTURE_PANE_OUTPUT}"
      fi
      exit "${TMUX_STUB_CAPTURE_PANE_EXIT}"
    fi

    if [ -n "${TMUX_STUB_CAPTURE_PANE_OUTPUT:-}" ]; then
      printf '%s' "${TMUX_STUB_CAPTURE_PANE_OUTPUT}"
    fi
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(tmuxPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write tmux stub: %v", err)
	}
	if err := os.Chmod(tmuxPath, 0o755); err != nil {
		t.Fatalf("chmod tmux stub: %v", err)
	}

	t.Setenv("PATH", dir)
	t.Setenv("TMUX_STUB_LOG", logPath)
	t.Setenv("TMUX_STUB_HAS_SESSION_EXIT", "")
	t.Setenv("TMUX_STUB_MISSING_SESSION", "")
	t.Setenv("TMUX_STUB_SEND_KEYS_FAIL_ON", "")
	t.Setenv("TMUX_STUB_SEND_KEYS_STDERR", "")
	t.Setenv("TMUX_STUB_CAPTURE_PANE_EXIT", "")
	t.Setenv("TMUX_STUB_CAPTURE_PANE_STDERR", "")
	t.Setenv("TMUX_STUB_CAPTURE_PANE_OUTPUT", "")

	return dir, logPath
}

func setupNoTmux(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func readLogLines(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		t.Fatalf("read log: %v", err)
	}
	out := strings.TrimSpace(string(data))
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestTmuxAvailable(t *testing.T) {
	cases := []struct {
		name     string
		withStub bool
		want     bool
	}{
		{name: "missing", withStub: false, want: false},
		{name: "present", withStub: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.withStub {
				setupStubTmux(t)
			} else {
				setupNoTmux(t)
			}

			if got := TmuxAvailable(); got != tc.want {
				t.Fatalf("TmuxAvailable()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestHasSession(t *testing.T) {
	cases := []struct {
		name           string
		withStub       bool
		session        string
		missingSession string
		exitCode       string
		want           bool
		wantErr        bool
	}{
		{name: "tmux missing", withStub: false, session: "s", want: false, wantErr: true},
		{name: "session exists", withStub: true, session: "s", want: true, wantErr: false},
		{name: "session missing", withStub: true, session: "s", missingSession: "s", want: false, wantErr: false},
		{name: "has-session errors", withStub: true, session: "s", exitCode: "2", want: false, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.withStub {
				setupStubTmux(t)
				t.Setenv("TMUX_STUB_MISSING_SESSION", tc.missingSession)
				t.Setenv("TMUX_STUB_HAS_SESSION_EXIT", tc.exitCode)
			} else {
				setupNoTmux(t)
			}

			got, err := HasSession(tc.session)
			if (err != nil) != tc.wantErr {
				t.Fatalf("HasSession() err=%v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !tc.withStub {
				if !errors.Is(err, ErrTmuxNotAvailable) {
					t.Fatalf("HasSession() err=%v, want %v", err, ErrTmuxNotAvailable)
				}
			}
			if got != tc.want {
				t.Fatalf("HasSession()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestSendKeys(t *testing.T) {
	cases := []struct {
		name         string
		withStub     bool
		session      string
		text         string
		failOn       string
		failStderr   string
		wantErr      bool
		wantContains string
		wantLog      []string
	}{
		{name: "tmux missing", withStub: false, session: "s", text: "hello", wantErr: true},
		{
			name:     "success",
			withStub: true,
			session:  "s",
			text:     "hello",
			wantErr:  false,
			wantLog: []string{
				"send-keys -t s:0.0 hello",
				"send-keys -t s:0.0 Enter",
			},
		},
		{
			name:         "text send fails",
			withStub:     true,
			session:      "s",
			text:         "hello",
			failOn:       "hello",
			failStderr:   "boom",
			wantErr:      true,
			wantContains: "tmux send-keys failed",
		},
		{
			name:         "enter send fails",
			withStub:     true,
			session:      "s",
			text:         "hello",
			failOn:       "Enter",
			failStderr:   "enterfail",
			wantErr:      true,
			wantContains: "tmux send-keys (Enter) failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logPath := ""
			if tc.withStub {
				_, logPath = setupStubTmux(t)
				t.Setenv("TMUX_STUB_SEND_KEYS_FAIL_ON", tc.failOn)
				t.Setenv("TMUX_STUB_SEND_KEYS_STDERR", tc.failStderr)
			} else {
				setupNoTmux(t)
			}

			err := SendKeys(tc.session, tc.text)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SendKeys() err=%v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !tc.withStub {
				if !errors.Is(err, ErrTmuxNotAvailable) {
					t.Fatalf("SendKeys() err=%v, want %v", err, ErrTmuxNotAvailable)
				}
			}
			if tc.wantContains != "" && (err == nil || !strings.Contains(err.Error(), tc.wantContains)) {
				t.Fatalf("SendKeys() err=%v, want contains %q", err, tc.wantContains)
			}

			if tc.withStub && len(tc.wantLog) > 0 {
				got := readLogLines(t, logPath)
				if len(got) != len(tc.wantLog) {
					t.Fatalf("tmux log lines=%d, want %d (%v)", len(got), len(tc.wantLog), got)
				}
				for i := range tc.wantLog {
					if got[i] != tc.wantLog[i] {
						t.Fatalf("tmux log[%d]=%q, want %q", i, got[i], tc.wantLog[i])
					}
				}
			}
		})
	}
}

func TestCapturePane(t *testing.T) {
	cases := []struct {
		name           string
		withStub       bool
		session        string
		lines          int
		out            string
		exitCode       string
		stderr         string
		want           string
		wantErr        bool
		wantContains   string
		wantLogContain string
	}{
		{name: "tmux missing", withStub: false, session: "s", lines: 0, want: "", wantErr: true},
		{
			name:           "success with lines",
			withStub:       true,
			session:        "s",
			lines:          5,
			out:            "pane\n",
			want:           "pane\n",
			wantErr:        false,
			wantLogContain: "capture-pane -p -t s:0.0 -S -5",
		},
		{
			name:         "capture-pane error with stderr",
			withStub:     true,
			session:      "s",
			lines:        0,
			exitCode:     "1",
			stderr:       "no pane",
			want:         "",
			wantErr:      true,
			wantContains: "tmux capture-pane failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logPath := ""
			if tc.withStub {
				_, logPath = setupStubTmux(t)
				t.Setenv("TMUX_STUB_CAPTURE_PANE_OUTPUT", tc.out)
				t.Setenv("TMUX_STUB_CAPTURE_PANE_EXIT", tc.exitCode)
				t.Setenv("TMUX_STUB_CAPTURE_PANE_STDERR", tc.stderr)
			} else {
				setupNoTmux(t)
			}

			got, err := CapturePane(tc.session, tc.lines)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CapturePane() err=%v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !tc.withStub {
				if !errors.Is(err, ErrTmuxNotAvailable) {
					t.Fatalf("CapturePane() err=%v, want %v", err, ErrTmuxNotAvailable)
				}
			}
			if tc.wantContains != "" && (err == nil || !strings.Contains(err.Error(), tc.wantContains)) {
				t.Fatalf("CapturePane() err=%v, want contains %q", err, tc.wantContains)
			}
			if got != tc.want {
				t.Fatalf("CapturePane()=%q, want %q", got, tc.want)
			}

			if tc.withStub && tc.wantLogContain != "" {
				lines := readLogLines(t, logPath)
				if len(lines) == 0 {
					t.Fatalf("expected tmux log entry, got none")
				}
				found := false
				for _, line := range lines {
					if strings.Contains(line, tc.wantLogContain) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("tmux log missing %q; got %v", tc.wantLogContain, lines)
				}
			}
		})
	}
}

func TestWaitFor(t *testing.T) {
	cases := []struct {
		name         string
		withStub     bool
		session      string
		pattern      string
		timeout      time.Duration
		lines        int
		captureOut   string
		captureExit  string
		captureErr   string
		wantErr      bool
		wantContains string
		wantOut      string
	}{
		{name: "tmux missing", withStub: false, session: "s", pattern: "needle", timeout: 10 * time.Millisecond, wantErr: true},
		{
			name:         "pattern required",
			withStub:     true,
			session:      "s",
			pattern:      "   ",
			timeout:      10 * time.Millisecond,
			wantErr:      true,
			wantContains: "pattern is required",
		},
		{
			name:       "immediate match",
			withStub:   true,
			session:    "s",
			pattern:    "needle",
			timeout:    50 * time.Millisecond,
			captureOut: "hello needle\n",
			wantErr:    false,
			wantOut:    "hello needle\n",
		},
		{
			name:         "timeout",
			withStub:     true,
			session:      "s",
			pattern:      "needle",
			timeout:      10 * time.Millisecond,
			captureOut:   "no match\n",
			wantErr:      true,
			wantContains: "timeout waiting for",
			wantOut:      "no match\n",
		},
		{
			name:         "capture-pane error",
			withStub:     true,
			session:      "s",
			pattern:      "needle",
			timeout:      10 * time.Millisecond,
			captureExit:  "1",
			captureErr:   "pane failed",
			wantErr:      true,
			wantContains: "tmux capture-pane failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.withStub {
				setupStubTmux(t)
				t.Setenv("TMUX_STUB_CAPTURE_PANE_OUTPUT", tc.captureOut)
				t.Setenv("TMUX_STUB_CAPTURE_PANE_EXIT", tc.captureExit)
				t.Setenv("TMUX_STUB_CAPTURE_PANE_STDERR", tc.captureErr)
			} else {
				setupNoTmux(t)
			}

			got, err := WaitFor(tc.session, tc.pattern, tc.timeout, tc.lines)
			if (err != nil) != tc.wantErr {
				t.Fatalf("WaitFor() err=%v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !tc.withStub {
				if !errors.Is(err, ErrTmuxNotAvailable) {
					t.Fatalf("WaitFor() err=%v, want %v", err, ErrTmuxNotAvailable)
				}
			}
			if tc.wantContains != "" && (err == nil || !strings.Contains(err.Error(), tc.wantContains)) {
				t.Fatalf("WaitFor() err=%v, want contains %q", err, tc.wantContains)
			}
			if tc.wantOut != "" && got != tc.wantOut {
				t.Fatalf("WaitFor()=%q, want %q", got, tc.wantOut)
			}
		})
	}
}

