package mcp

import (
	"strings"
	"testing"
	"time"
)

func TestDepWaitForDependenciesImmediate(t *testing.T) {
	s := &Server{
		tracked: map[string]map[int]trackedAgent{
			"ws": {
				1: {agentType: "a", tmuxTarget: "t1"},
				2: {agentType: "b", tmuxTarget: "t2"},
			},
		},
		nextSlot:        map[string]int{},
		depPollInterval: 5 * time.Millisecond,
		targetExistsFn: func(string) bool {
			return true
		},
		idleCheckFn: func(target, agentType, workspace string, slot int) bool {
			return true
		},
	}

	start := time.Now()
	if err := s.waitForDependencies("ws", []int{1, 2, 2}, 1); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("expected immediate return, took %s", time.Since(start))
	}
}

func TestDepWaitForDependenciesWaitsUntilIdle(t *testing.T) {
	start := time.Now()
	s := &Server{
		tracked: map[string]map[int]trackedAgent{
			"ws": {
				0: {agentType: "a", tmuxTarget: "t0"},
			},
		},
		nextSlot:        map[string]int{},
		depPollInterval: 5 * time.Millisecond,
		targetExistsFn: func(string) bool {
			return true
		},
		idleCheckFn: func(target, agentType, workspace string, slot int) bool {
			// Become idle shortly after start.
			return time.Since(start) >= 15*time.Millisecond
		},
	}

	if err := s.waitForDependencies("ws", []int{0}, 1); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if time.Since(start) < 10*time.Millisecond {
		t.Fatalf("expected to wait at least briefly, returned in %s", time.Since(start))
	}
}

func TestDepWaitForDependenciesMissingSlotErrors(t *testing.T) {
	s := &Server{
		tracked:         map[string]map[int]trackedAgent{"ws": {}},
		nextSlot:        map[string]int{},
		depPollInterval: 5 * time.Millisecond,
		targetExistsFn:  func(string) bool { return true },
		idleCheckFn:     func(string, string, string, int) bool { return true },
	}

	err := s.waitForDependencies("ws", []int{123}, 1)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not tracked") {
		t.Fatalf("expected not tracked error, got %v", err)
	}
}

func TestDepWaitForDependenciesKilledSlotErrors(t *testing.T) {
	s := &Server{
		tracked: map[string]map[int]trackedAgent{
			"ws": {
				1: {agentType: "a", tmuxTarget: "dead"},
			},
		},
		nextSlot:        map[string]int{},
		depPollInterval: 5 * time.Millisecond,
		targetExistsFn: func(target string) bool {
			return target != "dead"
		},
		idleCheckFn: func(string, string, string, int) bool { return true },
	}

	err := s.waitForDependencies("ws", []int{1}, 1)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not alive") && !strings.Contains(err.Error(), "killed") {
		t.Fatalf("expected killed/not alive error, got %v", err)
	}
}

func TestDepWaitForDependenciesTimeout(t *testing.T) {
	start := time.Now()
	s := &Server{
		tracked: map[string]map[int]trackedAgent{
			"ws": {
				0: {agentType: "a", tmuxTarget: "t0"},
			},
		},
		nextSlot:        map[string]int{},
		depPollInterval: 10 * time.Millisecond,
		targetExistsFn:  func(string) bool { return true },
		idleCheckFn:     func(string, string, string, int) bool { return false },
	}

	err := s.waitForDependencies("ws", []int{0}, 1)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Fatalf("expected to wait close to timeout, returned in %s", time.Since(start))
	}
}
