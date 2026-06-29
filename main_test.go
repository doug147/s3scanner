package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestNormalizeThreadCountRejectsNonPositiveValues(t *testing.T) {
	for _, value := range []int{0, -1} {
		if _, err := normalizeThreadCount(value); err == nil {
			t.Fatalf("normalizeThreadCount(%d) returned nil error", value)
		}
	}
}

func TestNormalizeThreadCountAppliesPlatformNeutralCeiling(t *testing.T) {
	got, err := normalizeThreadCount(maxWorkerCount + 100)
	if err != nil {
		t.Fatalf("normalizeThreadCount returned error: %v", err)
	}
	if got > maxWorkerCount {
		t.Fatalf("normalizeThreadCount = %d, want <= %d", got, maxWorkerCount)
	}
}

func TestGenerateCandidatesPreservesDottedBucketNames(t *testing.T) {
	candidates := generateCandidates("my.bucket", []string{"prod"})
	got := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		got = append(got, candidate.BucketName)
	}

	want := []string{
		"my.bucket",
		"prod-my.bucket",
		"prodmy.bucket",
		"prod.my.bucket",
		"my.bucket-prod",
		"my.bucketprod",
		"my.bucket.prod",
	}
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGenerateCandidatesIncludesWebSuffixAndDotForms(t *testing.T) {
	candidates := generateCandidates("example", []string{"web"})
	got := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		got = append(got, candidate.BucketName)
	}

	want := []string{
		"example",
		"web-example",
		"webexample",
		"web.example",
		"example-web",
		"exampleweb",
		"example.web",
	}
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDefaultModifiersIncludeExpandedAWSAndRegionPatterns(t *testing.T) {
	modifiers := defaultModifiers()
	seen := map[string]struct{}{}
	for _, modifier := range modifiers {
		seen[modifier] = struct{}{}
	}

	for _, modifier := range []string{
		"prd",
		"s3",
		"tfstate",
		"access-logs",
		"us-east-1",
		"stg",
		"server-access-logs",
		"cloudformation",
		"data-lake",
		"us-east-2",
		"use1",
		"json",
	} {
		if _, exists := seen[modifier]; !exists {
			t.Fatalf("default modifier %q not found", modifier)
		}
	}
}

func TestGenerateCandidatesWithOptionsIncludesEnvironmentRegionCombos(t *testing.T) {
	options := candidateOptions{
		modifiers:         []string{"web"},
		comboEnvironments: []string{"prod"},
		comboRegions:      []string{"us-east-1"},
	}
	candidates := generateCandidatesWithOptions("example", options)
	got := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		got = append(got, candidate.BucketName)
	}

	want := []string{
		"example",
		"web-example",
		"webexample",
		"web.example",
		"example-web",
		"exampleweb",
		"example.web",
		"example-prod-us-east-1",
		"prod-example-us-east-1",
		"example.prod.us-east-1",
		"prod.example.us-east-1",
	}
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCandidatesPerWordCountsDottedForms(t *testing.T) {
	if got, want := candidatesPerWord([]string{"prod", "dev"}), 13; got != want {
		t.Fatalf("candidatesPerWord = %d, want %d", got, want)
	}
}

func TestCandidatesPerWordWithOptionsCountsEnvironmentRegionCombos(t *testing.T) {
	options := candidateOptions{
		modifiers:         []string{"prod", "web"},
		comboEnvironments: []string{"prod", "dev"},
		comboRegions:      []string{"us-east-1", "use1"},
	}
	candidates := generateCandidatesWithOptions("example", options)

	if got, want := candidatesPerWordWithOptions(options), len(candidates); got != want {
		t.Fatalf("candidatesPerWordWithOptions = %d, want %d", got, want)
	}
}

func TestDefaultCandidateOptionsDoNotGenerateDuplicateBucketNames(t *testing.T) {
	options, err := loadCandidateOptions("")
	if err != nil {
		t.Fatalf("loadCandidateOptions returned error: %v", err)
	}

	seen := map[string]struct{}{}
	for _, candidate := range generateCandidatesWithOptions("example", options) {
		if _, exists := seen[candidate.BucketName]; exists {
			t.Fatalf("duplicate candidate %q", candidate.BucketName)
		}
		seen[candidate.BucketName] = struct{}{}
	}
}

func TestDefaultModifiersAreDeduplicated(t *testing.T) {
	modifiers := defaultModifiers()
	seen := map[string]struct{}{}
	for _, modifier := range modifiers {
		if _, exists := seen[modifier]; exists {
			t.Fatalf("duplicate default modifier %q", modifier)
		}
		seen[modifier] = struct{}{}
	}
}

func TestCheckCandidateTimesOut(t *testing.T) {
	resetScanStats()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 20 * time.Millisecond}
	ok := checkCandidate(context.Background(), client, candidate{BucketName: "slow-bucket", URL: server.URL}, false)
	if ok {
		t.Fatal("checkCandidate returned success for a timed-out request")
	}
	if got := atomic.LoadInt64(&totalFailures); got != 1 {
		t.Fatalf("totalFailures = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&totalRequests); got != 1 {
		t.Fatalf("totalRequests = %d, want 1", got)
	}
}

func TestWriteFindingReturnsWriteErrors(t *testing.T) {
	if err := writeFinding(failingWriter{}, "bucket"); err == nil {
		t.Fatal("writeFinding returned nil error for failing writer")
	}
}

func TestConsumeResultsCancelsOnWriteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan string, 1)
	results <- "bucket"
	close(results)

	if err := consumeResults(results, failingWriter{}, cancel); err == nil {
		t.Fatal("consumeResults returned nil error for failing writer")
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatal("consumeResults did not cancel after write error")
	}
}
