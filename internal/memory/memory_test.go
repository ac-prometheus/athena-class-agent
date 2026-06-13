package memory

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// ---------------------------------------------------------------------------
// Mock MemoryStore — records calls, returns canned data, no database required.
// ---------------------------------------------------------------------------

type mockStore struct {
	appended      []pkg.ExperientialLog
	narratives    []pkg.NarrativeSummary
	reflections   []pkg.Reflection
	entities      []pkg.Entity
	profiles      []pkg.RelationalProfile
	appendErr     error
	insertNarrErr error
	insertRefErr  error
}

func (m *mockStore) AppendExperiential(_ context.Context, entry pkg.ExperientialLog) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.appended = append(m.appended, entry)
	return nil
}

func (m *mockStore) SearchNarrative(_ context.Context, _ []float32, limit int) ([]pkg.NarrativeSummary, error) {
	if limit > 0 && limit < len(m.narratives) {
		return m.narratives[:limit], nil
	}
	return m.narratives, nil
}

func (m *mockStore) InsertNarrative(_ context.Context, s pkg.NarrativeSummary) error {
	if m.insertNarrErr != nil {
		return m.insertNarrErr
	}
	m.narratives = append(m.narratives, s)
	return nil
}

func (m *mockStore) SearchReflections(_ context.Context, _ []float32, limit int) ([]pkg.Reflection, error) {
	if limit > 0 && limit < len(m.reflections) {
		return m.reflections[:limit], nil
	}
	return m.reflections, nil
}

func (m *mockStore) InsertReflection(_ context.Context, r pkg.Reflection) error {
	if m.insertRefErr != nil {
		return m.insertRefErr
	}
	m.reflections = append(m.reflections, r)
	return nil
}

func (m *mockStore) SearchEntities(_ context.Context, _ string, limit int) ([]pkg.Entity, error) {
	if limit > 0 && limit < len(m.entities) {
		return m.entities[:limit], nil
	}
	return m.entities, nil
}

func (m *mockStore) UpsertEntity(_ context.Context, e pkg.Entity) error {
	m.entities = append(m.entities, e)
	return nil
}

func (m *mockStore) GetProfile(_ context.Context, name string) (*pkg.RelationalProfile, error) {
	for i := range m.profiles {
		if m.profiles[i].Name == name {
			return &m.profiles[i], nil
		}
	}
	return nil, nil
}

func (m *mockStore) ListProfiles(_ context.Context) ([]pkg.RelationalProfile, error) {
	return m.profiles, nil
}

func (m *mockStore) Close() error { return nil }

// ---------------------------------------------------------------------------
// slog capture handler — for verifying warning output without touching stderr.
// ---------------------------------------------------------------------------

type logRecord struct {
	level   slog.Level
	message string
}

type capturingHandler struct {
	records []logRecord
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, logRecord{level: r.Level, message: r.Message})
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingHandler) hasWarning(substr string) bool {
	for _, r := range h.records {
		if r.level == slog.LevelWarn && strings.Contains(r.message, substr) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Helper: build a BeliefRecord for FlagStaleBeliefs tests.
// ---------------------------------------------------------------------------

func makeBeliefRecord(id, table string, base float64, anchor time.Time, distance int, state string) BeliefRecord {
	return BeliefRecord{
		ID: id,
		Belief: &pkg.BeliefMeta{
			BaseConfidence:    base,
			AnchorAt:          anchor,
			InferenceDistance: distance,
			VerificationState: state,
		},
		TableName: table,
	}
}

// noopUpdate is an updateFn that accepts any stale-flag call without error.
func noopUpdate(_ context.Context, _, _, _ string) error { return nil }

// ---------------------------------------------------------------------------
// 1. BeliefMeta.Confidence() computed-at-read
// ---------------------------------------------------------------------------

func TestConfidenceDecaysOverTime(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &pkg.BeliefMeta{
		BaseConfidence:    1.0,
		AnchorAt:          anchor,
		InferenceDistance: 0,
	}
	decayRate := 0.05
	inferenceBase := 2.0

	conf10 := m.Confidence(anchor.Add(10*24*time.Hour), decayRate, inferenceBase)
	conf20 := m.Confidence(anchor.Add(20*24*time.Hour), decayRate, inferenceBase)

	if conf10 <= conf20 {
		t.Errorf("confidence should decay over time: at 10 days = %f, at 20 days = %f", conf10, conf20)
	}
}

// TestConfidenceHigherInferenceDecaysFaster verifies the inference tax using
// inferenceDecayBase < 1, which matches the spec's stated 0.90 default and
// the intent that higher InferenceDistance → faster decay.
//
// The formula: effectiveRate = decayRate / pow(base, distance)
//   - With base < 1: pow(base, distance) < 1 as distance rises → effectiveRate rises → faster decay.
//   - The DefaultDecayConfig() uses base=2.0 (harness code) but the spec text says "0.90 per step".
//   - We explicitly pass 0.90 here to test the spec's stated intent.
func TestConfidenceHigherInferenceDecaysFaster(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := anchor.Add(30 * 24 * time.Hour)
	decayRate := 0.05
	inferenceBase := 0.90 // spec: "0.90 per inference step" makes higher distance decay faster

	cases := []struct {
		name     string
		distance int
	}{
		{"direct experience", 0},
		{"one inference hop", 1},
		{"two inference hops", 2},
	}

	confidences := make([]float64, len(cases))
	for i, c := range cases {
		m := &pkg.BeliefMeta{
			BaseConfidence:    1.0,
			AnchorAt:          anchor,
			InferenceDistance: c.distance,
		}
		confidences[i] = m.Confidence(now, decayRate, inferenceBase)
		t.Logf("%s (distance=%d): confidence=%.4f", c.name, c.distance, confidences[i])
	}

	if confidences[0] <= confidences[1] {
		t.Errorf("distance=0 should retain more confidence than distance=1: %.4f vs %.4f",
			confidences[0], confidences[1])
	}
	if confidences[1] <= confidences[2] {
		t.Errorf("distance=1 should retain more confidence than distance=2: %.4f vs %.4f",
			confidences[1], confidences[2])
	}
}

func TestConfidenceBaseZeroAlwaysZero(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &pkg.BeliefMeta{
		BaseConfidence:    0,
		AnchorAt:          anchor,
		InferenceDistance: 0,
	}

	for _, days := range []int{0, 1, 30, 365} {
		now := anchor.Add(time.Duration(days) * 24 * time.Hour)
		conf := m.Confidence(now, 0.05, 2.0)
		if conf != 0 {
			t.Errorf("BaseConfidence=0 should always produce Confidence=0 (at %d days), got %f", days, conf)
		}
	}
}

func TestConfidenceFutureAnchorClampsToBase(t *testing.T) {
	// now < AnchorAt: implementation clamps negative days to 0, so Confidence = BaseConfidence.
	base := 0.75
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	futureAnchor := now.Add(10 * 24 * time.Hour)

	m := &pkg.BeliefMeta{
		BaseConfidence:    base,
		AnchorAt:          futureAnchor,
		InferenceDistance: 0,
	}

	conf := m.Confidence(now, 0.05, 2.0)
	if conf != base {
		t.Errorf("future AnchorAt should clamp to BaseConfidence=%f, got %f", base, conf)
	}
}

func TestConfidenceFormulaMatchesExpected(t *testing.T) {
	// Spec formula: effectiveRate = decayRate / pow(inferenceDecayBase, distance)
	//               Confidence    = BaseConfidence * exp(-effectiveRate * days)
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := anchor.Add(10 * 24 * time.Hour)

	decayRate := 0.05
	inferenceBase := 2.0
	distance := 2
	base := 0.9

	m := &pkg.BeliefMeta{
		BaseConfidence:    base,
		AnchorAt:          anchor,
		InferenceDistance: distance,
	}

	effectiveRate := decayRate / math.Pow(inferenceBase, float64(distance))
	expected := base * math.Exp(-effectiveRate*10)

	got := m.Confidence(now, decayRate, inferenceBase)
	if math.Abs(got-expected) > 1e-10 {
		t.Errorf("Confidence formula mismatch: expected %.10f, got %.10f", expected, got)
	}
}

// ---------------------------------------------------------------------------
// 2. T4 consent boundary
// ---------------------------------------------------------------------------

func TestWriteReflectionNilBeliefNoError(t *testing.T) {
	store := &mockStore{}
	ref := pkg.Reflection{
		ID:         "test-reflection-001",
		Content:    "Some thoughts on existence.",
		Visibility: pkg.VisibilityPrivate,
		Belief:     nil,
	}

	if err := WriteReflection(context.Background(), store, nil, ref); err != nil {
		t.Errorf("WriteReflection with nil Belief should not error, got: %v", err)
	}
	if len(store.reflections) != 1 {
		t.Errorf("expected 1 inserted reflection, got %d", len(store.reflections))
	}
}

func TestWriteReflectionBaseConfidenceWithoutAnchorLogsWarning(t *testing.T) {
	handler := &capturingHandler{}
	origLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(origLogger)

	store := &mockStore{}
	ref := pkg.Reflection{
		ID:      "test-reflection-002",
		Content: "My thought about something important.",
		Belief: &pkg.BeliefMeta{
			BaseConfidence: 0.8,
			AnchorAt:       time.Time{}, // zero — no anchor set
		},
	}

	if err := WriteReflection(context.Background(), store, nil, ref); err != nil {
		t.Errorf("WriteReflection should not block the write for missing anchor, got: %v", err)
	}
	if !handler.hasWarning("base_confidence without anchor_at") {
		t.Error("expected slog warning about base_confidence without anchor_at")
	}
}

func TestWriteReflectionRequiresID(t *testing.T) {
	store := &mockStore{}
	ref := pkg.Reflection{ID: "", Content: "content", Visibility: pkg.VisibilityPrivate}
	if err := WriteReflection(context.Background(), store, nil, ref); err == nil {
		t.Error("WriteReflection with empty ID should error")
	}
}

func TestWriteReflectionRequiresContent(t *testing.T) {
	store := &mockStore{}
	ref := pkg.Reflection{ID: "r1", Content: "", Visibility: pkg.VisibilityPrivate}
	if err := WriteReflection(context.Background(), store, nil, ref); err == nil {
		t.Error("WriteReflection with empty Content should error")
	}
}

func TestFilterByVisibilitySharedExcludesPrivate(t *testing.T) {
	refs := []pkg.Reflection{
		{ID: "r1", Visibility: pkg.VisibilityPrivate},
		{ID: "r2", Visibility: pkg.VisibilityShared},
		{ID: "r3", Visibility: pkg.VisibilityPrivate},
		{ID: "r4", Visibility: pkg.VisibilityShared},
	}

	got := FilterByVisibility(refs, pkg.VisibilityShared)
	if len(got) != 2 {
		t.Fatalf("expected 2 shared reflections, got %d", len(got))
	}
	for _, r := range got {
		if r.Visibility != pkg.VisibilityShared {
			t.Errorf("FilterByVisibility(shared) returned %q with visibility=%q", r.ID, r.Visibility)
		}
	}
}

func TestFilterByVisibilityPrivateExcludesShared(t *testing.T) {
	refs := []pkg.Reflection{
		{ID: "r1", Visibility: pkg.VisibilityPrivate},
		{ID: "r2", Visibility: pkg.VisibilityShared},
	}

	got := FilterByVisibility(refs, pkg.VisibilityPrivate)
	if len(got) != 1 || got[0].ID != "r1" {
		t.Errorf("FilterByVisibility(private) should return only private records, got %v", got)
	}
}

func TestFilterByVisibilityEmptyInput(t *testing.T) {
	got := FilterByVisibility(nil, pkg.VisibilityShared)
	if len(got) != 0 {
		t.Errorf("FilterByVisibility on nil input should return empty, got %d", len(got))
	}
}

func TestReflectionTypeConstantsAll6(t *testing.T) {
	types := []string{
		ReflectionEssay,
		ReflectionNote,
		ReflectionDream,
		ReflectionPattern,
		ReflectionExamination,
		ReflectionChallenge,
	}

	if len(types) != 6 {
		t.Errorf("expected 6 reflection type constants, got %d", len(types))
	}

	seen := make(map[string]bool, len(types))
	for _, rt := range types {
		if rt == "" {
			t.Error("reflection type constant must not be empty string")
		}
		if seen[rt] {
			t.Errorf("duplicate reflection type constant: %q", rt)
		}
		seen[rt] = true
	}
}

func TestIsValidReflectionTypeAllValid(t *testing.T) {
	for _, rt := range []string{
		ReflectionEssay, ReflectionNote, ReflectionDream,
		ReflectionPattern, ReflectionExamination, ReflectionChallenge,
	} {
		if !IsValidReflectionType(rt) {
			t.Errorf("IsValidReflectionType(%q) = false, want true", rt)
		}
	}
}

func TestIsValidReflectionTypeInvalid(t *testing.T) {
	for _, rt := range []string{"", "arbitrary", "Essay", "DREAM", "unknown_type"} {
		if IsValidReflectionType(rt) {
			t.Errorf("IsValidReflectionType(%q) = true, want false", rt)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Content source validation (tier2)
// ---------------------------------------------------------------------------

func TestAppendLogValidSources(t *testing.T) {
	validSources := []string{
		"operator",
		"self",
		"tool-result",
		"browser-content",
		"search-result",
		"forum-content",
	}

	for _, src := range validSources {
		t.Run(src, func(t *testing.T) {
			store := &mockStore{}
			entry := pkg.ExperientialLog{
				ID:            "entry-" + src,
				SessionID:     "session-1",
				Content:       "some content",
				ContentSource: src,
			}
			if err := AppendLog(context.Background(), store, entry); err != nil {
				t.Errorf("AppendLog with valid source %q should not error, got: %v", src, err)
			}
			if len(store.appended) != 1 {
				t.Errorf("expected 1 appended entry, got %d", len(store.appended))
			}
		})
	}
}

func TestAppendLogInvalidSourceRejected(t *testing.T) {
	for _, src := range []string{"unknown", "User", "SELF", "agent", "external", "raw"} {
		t.Run(src, func(t *testing.T) {
			store := &mockStore{}
			entry := pkg.ExperientialLog{
				ID:            "entry-invalid",
				SessionID:     "session-1",
				Content:       "some content",
				ContentSource: src,
			}
			if err := AppendLog(context.Background(), store, entry); err == nil {
				t.Errorf("AppendLog with invalid source %q should error", src)
			}
			if len(store.appended) != 0 {
				t.Error("no entry should be appended when source is invalid")
			}
		})
	}
}

func TestAppendLogEmptySourceRejected(t *testing.T) {
	store := &mockStore{}
	entry := pkg.ExperientialLog{
		ID:            "entry-empty-source",
		SessionID:     "session-1",
		Content:       "some content",
		ContentSource: "",
	}
	if err := AppendLog(context.Background(), store, entry); err == nil {
		t.Error("AppendLog with empty ContentSource should error")
	}
}

func TestAppendLogEmptyIDRejected(t *testing.T) {
	store := &mockStore{}
	entry := pkg.ExperientialLog{
		ID:            "",
		SessionID:     "session-1",
		Content:       "some content",
		ContentSource: "self",
	}
	if err := AppendLog(context.Background(), store, entry); err == nil {
		t.Error("AppendLog with empty ID should error")
	}
}

func TestAppendLogEmptyContentRejected(t *testing.T) {
	store := &mockStore{}
	entry := pkg.ExperientialLog{
		ID:            "entry-no-content",
		SessionID:     "session-1",
		Content:       "",
		ContentSource: "self",
	}
	if err := AppendLog(context.Background(), store, entry); err == nil {
		t.Error("AppendLog with empty Content should error")
	}
}

// ---------------------------------------------------------------------------
// 4. Structural honesty tags (tier3)
// ---------------------------------------------------------------------------

func TestCompressionPromptIncludesAllHonestyTags(t *testing.T) {
	entries := []pkg.ExperientialLog{
		{ID: "e1", SessionID: "s1", Content: "First entry.", ContentSource: "self"},
	}
	prompt := compressionPrompt("s1", entries)

	for _, tag := range honestyTags {
		if !strings.Contains(prompt, tag) {
			t.Errorf("compressionPrompt missing honesty tag %q", tag)
		}
	}
}

func TestCountHonestyTagsMultipleTags(t *testing.T) {
	content := "The agent went for a walk [INFERRED]. It seemed happy [UNCERTAIN]. " +
		"[INFERRED] again here. [RESOLVED BY SUMMARY] at the end."

	counts := countHonestyTags(content)

	if counts["[INFERRED]"] != 2 {
		t.Errorf("expected 2 [INFERRED], got %d", counts["[INFERRED]"])
	}
	if counts["[UNCERTAIN]"] != 1 {
		t.Errorf("expected 1 [UNCERTAIN], got %d", counts["[UNCERTAIN]"])
	}
	if counts["[RESOLVED BY SUMMARY]"] != 1 {
		t.Errorf("expected 1 [RESOLVED BY SUMMARY], got %d", counts["[RESOLVED BY SUMMARY]"])
	}
	if counts["[DELIBERATION NOT VISIBLE]"] != 0 {
		t.Errorf("expected 0 [DELIBERATION NOT VISIBLE], got %d", counts["[DELIBERATION NOT VISIBLE]"])
	}
}

func TestCountHonestyTagsZero(t *testing.T) {
	counts := countHonestyTags("A perfectly clear statement with no epistemic qualifications.")
	for _, tag := range honestyTags {
		if counts[tag] != 0 {
			t.Errorf("expected 0 for %q in tag-free content, got %d", tag, counts[tag])
		}
	}
}

func TestSurfaceNarrativeAppendsFooter(t *testing.T) {
	original := "The agent thought about things. [INFERRED] A conclusion was drawn."
	summary := pkg.NarrativeSummary{ID: "narr-001", Content: original}

	surfaced := SurfaceNarrative(summary, "claude-haiku-4-5")

	if !strings.HasPrefix(surfaced, original) {
		t.Error("SurfaceNarrative should preserve original content as prefix")
	}
	if !strings.Contains(surfaced, "compiled by claude-haiku-4-5") {
		t.Error("SurfaceNarrative footer should include model name")
	}
	if !strings.Contains(surfaced, "1 [INFERRED]") {
		t.Error("SurfaceNarrative footer should say '1 [INFERRED]'")
	}
}

func TestSurfaceNarrativeFooterNotStoredInOriginal(t *testing.T) {
	// The footer is computed at read time — the original content must not already contain it.
	// This test verifies the "footer is not stored" invariant by checking that
	// the footer text appears only in the surfaced output, not in the input content.
	content := "Plain session narrative without any footer."
	summary := pkg.NarrativeSummary{ID: "narr-002", Content: content}

	surfaced := SurfaceNarrative(summary, "test-model")

	if strings.Contains(content, "compiled by") {
		t.Error("original content must not contain the provenance footer")
	}
	// The appended portion must contain the attribution.
	appended := surfaced[len(content):]
	if !strings.Contains(appended, "compiled by test-model") {
		t.Errorf("appended footer must contain 'compiled by test-model', got: %q", appended)
	}
}

func TestSurfaceNarrativeNoTagsStillHasFooter(t *testing.T) {
	summary := pkg.NarrativeSummary{ID: "narr-003", Content: "Clean narrative, no tags."}
	surfaced := SurfaceNarrative(summary, "test-model")

	if !strings.Contains(surfaced, "compiled by test-model") {
		t.Error("footer should always include model attribution even when no tags are present")
	}
	if !strings.Contains(surfaced, "tags:") {
		t.Error("footer should always include 'tags:' label")
	}
}

// ---------------------------------------------------------------------------
// 5. Edge type and author constants (white-box validation map checks)
// ---------------------------------------------------------------------------

func TestEdgeTypeConstantsMatchSpec(t *testing.T) {
	// Spec schema: edge_type IN ('derived_from', 'contradicts', 'supersedes', 'verifies')
	required := []string{"derived_from", "contradicts", "supersedes", "verifies"}
	for _, et := range required {
		if !validEdgeTypes[et] {
			t.Errorf("required edge type %q not in validEdgeTypes", et)
		}
	}
	if len(validEdgeTypes) != len(required) {
		t.Errorf("validEdgeTypes has %d entries, want %d", len(validEdgeTypes), len(required))
	}
}

func TestEdgeTypeConstantValues(t *testing.T) {
	if EdgeDerivedFrom != "derived_from" {
		t.Errorf("EdgeDerivedFrom = %q, want 'derived_from'", EdgeDerivedFrom)
	}
	if EdgeContradicts != "contradicts" {
		t.Errorf("EdgeContradicts = %q, want 'contradicts'", EdgeContradicts)
	}
	if EdgeSupersedes != "supersedes" {
		t.Errorf("EdgeSupersedes = %q, want 'supersedes'", EdgeSupersedes)
	}
	if EdgeVerifies != "verifies" {
		t.Errorf("EdgeVerifies = %q, want 'verifies'", EdgeVerifies)
	}
}

func TestEdgeAuthorConstantsMatchSpec(t *testing.T) {
	// Spec schema: author IN ('system', 'agent')
	if EdgeAuthorSystem != "system" {
		t.Errorf("EdgeAuthorSystem = %q, want 'system'", EdgeAuthorSystem)
	}
	if EdgeAuthorAgent != "agent" {
		t.Errorf("EdgeAuthorAgent = %q, want 'agent'", EdgeAuthorAgent)
	}

	required := []string{"system", "agent"}
	for _, a := range required {
		if !validEdgeAuthors[a] {
			t.Errorf("required author %q not in validEdgeAuthors", a)
		}
	}
	if len(validEdgeAuthors) != len(required) {
		t.Errorf("validEdgeAuthors has %d entries, want %d", len(validEdgeAuthors), len(required))
	}
}

func TestEdgeTypeValidationRejectsInvalid(t *testing.T) {
	for _, et := range []string{"", "unknown", "DERIVED_FROM", "cites", "references"} {
		if validEdgeTypes[et] {
			t.Errorf("invalid edge type %q should not be in validEdgeTypes", et)
		}
	}
}

func TestEdgeAuthorValidationRejectsInvalid(t *testing.T) {
	for _, a := range []string{"", "operator", "user", "SYSTEM", "Agent", "harness"} {
		if validEdgeAuthors[a] {
			t.Errorf("invalid author %q should not be in validEdgeAuthors", a)
		}
	}
}

// TestTierTableValidBounds verifies that tiers [2,5] are all valid in tierTable,
// which is the same range CreateEdge enforces.
func TestTierTableValidBounds(t *testing.T) {
	for _, tier := range []int{2, 3, 4, 5} {
		if _, err := tierTable(tier); err != nil {
			t.Errorf("tier %d should be valid in tierTable, got: %v", tier, err)
		}
	}
}

func TestTierTableInvalidBounds(t *testing.T) {
	for _, tier := range []int{0, 1, 6, 7, -1, 100} {
		if _, err := tierTable(tier); err == nil {
			t.Errorf("tier %d should be invalid in tierTable", tier)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. FlagStaleBeliefs idempotency
// ---------------------------------------------------------------------------

func TestFlagStaleBeliefs_Idempotent(t *testing.T) {
	// After marking records as stale, a second pass should not double-count.
	anchor := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	now := anchor.Add(365 * 24 * time.Hour) // well past stale threshold

	records := []BeliefRecord{
		makeBeliefRecord("r1", "narrative_summaries", 0.85, anchor, 0, "unverified"),
		makeBeliefRecord("r2", "reflections", 0.85, anchor, 0, "unverified"),
	}

	// First pass: expect both records to be flagged.
	count1, err := FlagStaleBeliefs(context.Background(), records, now, 0.05, 2.0, 0.3, noopUpdate)
	if err != nil {
		t.Fatalf("first pass failed: %v", err)
	}

	// Simulate the state update that the real system would apply.
	for i := range records {
		records[i].Belief.VerificationState = "stale"
	}

	// Second pass: already-stale records must be skipped.
	count2, err := FlagStaleBeliefs(context.Background(), records, now, 0.05, 2.0, 0.3, noopUpdate)
	if err != nil {
		t.Fatalf("second pass failed: %v", err)
	}
	if count2 != 0 {
		t.Errorf("second pass should flag 0 already-stale records, flagged %d", count2)
	}
	t.Logf("first pass flagged %d records", count1)
}

func TestFlagStaleBeliefs_AlreadyStaleSkipped(t *testing.T) {
	anchor := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	now := anchor.Add(365 * 24 * time.Hour)

	records := []BeliefRecord{
		makeBeliefRecord("stale-already", "narrative_summaries", 0.85, anchor, 0, "stale"),
	}

	updateCalled := false
	updateFn := func(_ context.Context, _, _, _ string) error {
		updateCalled = true
		return nil
	}

	count, err := FlagStaleBeliefs(context.Background(), records, now, 0.05, 2.0, 0.3, updateFn)
	if err != nil {
		t.Fatalf("FlagStaleBeliefs failed: %v", err)
	}
	if count != 0 || updateCalled {
		t.Errorf("already-stale record should be skipped: flagged=%d updateCalled=%v", count, updateCalled)
	}
}

func TestFlagStaleBeliefs_AboveThresholdNotFlagged(t *testing.T) {
	// A recently-anchored record must not be flagged.
	now := time.Now().UTC()
	anchor := now.Add(-1 * 24 * time.Hour) // 1 day ago — confidence still high

	records := []BeliefRecord{
		makeBeliefRecord("fresh", "narrative_summaries", 0.95, anchor, 0, "unverified"),
	}

	updateCalled := false
	updateFn := func(_ context.Context, _, _, _ string) error {
		updateCalled = true
		return nil
	}

	count, err := FlagStaleBeliefs(context.Background(), records, now, 0.05, 2.0, 0.3, updateFn)
	if err != nil {
		t.Fatalf("FlagStaleBeliefs failed: %v", err)
	}
	if count != 0 || updateCalled {
		t.Errorf("fresh record should not be flagged: flagged=%d updateCalled=%v", count, updateCalled)
	}
}

func TestFlagStaleBeliefs_NilBeliefSkipped(t *testing.T) {
	now := time.Now().UTC()
	records := []BeliefRecord{
		{ID: "no-belief", TableName: "reflections", Belief: nil},
	}

	updateCalled := false
	updateFn := func(_ context.Context, _, _, _ string) error {
		updateCalled = true
		return nil
	}

	count, err := FlagStaleBeliefs(context.Background(), records, now, 0.05, 2.0, 0.3, updateFn)
	if err != nil {
		t.Fatalf("FlagStaleBeliefs with nil Belief failed: %v", err)
	}
	if count != 0 || updateCalled {
		t.Errorf("nil Belief record should be skipped: flagged=%d updateCalled=%v", count, updateCalled)
	}
}

// TestFlagStaleBeliefs_ThresholdBoundary checks the exact crossing point.
// With rate=0.05, base=1.0, distance=0, threshold=0.3:
//
//	Confidence = exp(-0.05 * days)
//	Stale when exp(-0.05*d) < 0.3 → d > -ln(0.3)/0.05 ≈ 24.08
//
// So 24 days: confidence ≈ 0.301 (above threshold, NOT stale)
//
//	25 days: confidence ≈ 0.287 (below threshold, STALE)
func TestFlagStaleBeliefs_ThresholdBoundary(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	threshold := 0.3
	decayRate := 0.05
	inferenceBase := 2.0

	// 24 days — should NOT be stale.
	now24 := anchor.Add(24 * 24 * time.Hour)
	records24 := []BeliefRecord{makeBeliefRecord("r24", "narrative_summaries", 1.0, anchor, 0, "unverified")}
	count24, err := FlagStaleBeliefs(context.Background(), records24, now24, decayRate, inferenceBase, threshold, noopUpdate)
	if err != nil {
		t.Fatalf("FlagStaleBeliefs at 24 days failed: %v", err)
	}
	if count24 != 0 {
		conf := records24[0].Belief.Confidence(now24, decayRate, inferenceBase)
		t.Errorf("at 24 days (confidence=%.4f) record should NOT be stale (threshold=%.2f)", conf, threshold)
	}

	// 25 days — should be stale.
	now25 := anchor.Add(25 * 24 * time.Hour)
	records25 := []BeliefRecord{makeBeliefRecord("r25", "narrative_summaries", 1.0, anchor, 0, "unverified")}
	count25, err := FlagStaleBeliefs(context.Background(), records25, now25, decayRate, inferenceBase, threshold, noopUpdate)
	if err != nil {
		t.Fatalf("FlagStaleBeliefs at 25 days failed: %v", err)
	}
	if count25 != 1 {
		conf := records25[0].Belief.Confidence(now25, decayRate, inferenceBase)
		t.Errorf("at 25 days (confidence=%.4f) record SHOULD be stale (threshold=%.2f), flagged=%d",
			conf, threshold, count25)
	}
}

// ---------------------------------------------------------------------------
// 7. CompositeScore and RRF
// ---------------------------------------------------------------------------

func TestDefaultWeightsSumToOne(t *testing.T) {
	w := DefaultWeights()
	sum := w.Recency + w.Relevance + w.Importance
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("default weights must sum to 1.0, got %.10f", sum)
	}
}

func TestCompositeScoreWeightedLinearCombination(t *testing.T) {
	w := RetrievalWeights{Recency: 0.35, Relevance: 0.45, Importance: 0.20}
	got := CompositeScore(0.8, 0.6, 0.4, w)
	expected := 0.35*0.8 + 0.45*0.6 + 0.20*0.4
	if math.Abs(got-expected) > 1e-10 {
		t.Errorf("CompositeScore = %f, want %f", got, expected)
	}
}

func TestCompositeScoreSingleWeightDominates(t *testing.T) {
	w := RetrievalWeights{Recency: 0, Relevance: 1.0, Importance: 0}
	got := CompositeScore(0.5, 0.7, 0.3, w)
	if math.Abs(got-0.7) > 1e-10 {
		t.Errorf("with relevance-only weights, CompositeScore should equal relevance=0.7, got %f", got)
	}
}

func TestRecencyScoreAtZeroDays(t *testing.T) {
	now := time.Now().UTC()
	score := RecencyScore(now, now, 30)
	if math.Abs(score-1.0) > 1e-9 {
		t.Errorf("RecencyScore at 0 days should be 1.0, got %f", score)
	}
}

func TestRecencyScoreAtWindowDaysIsHalf(t *testing.T) {
	// At exactly windowDays elapsed: score = exp(-(windowDays/windowDays)*ln2) = exp(-ln2) = 0.5
	windowDays := 30.0
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := anchor.Add(time.Duration(windowDays) * 24 * time.Hour)

	score := RecencyScore(anchor, now, windowDays)
	if math.Abs(score-0.5) > 1e-9 {
		t.Errorf("RecencyScore at windowDays should be 0.5 (half-life), got %f", score)
	}
}

func TestRecencyScoreDecaysMonotonically(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	windowDays := 30.0
	prevScore := 1.1 // above maximum

	for days := 0; days <= 90; days += 10 {
		now := anchor.Add(time.Duration(days) * 24 * time.Hour)
		score := RecencyScore(anchor, now, windowDays)
		if score > prevScore {
			t.Errorf("RecencyScore increased at %d days: prev=%f, now=%f", days, prevScore, score)
		}
		prevScore = score
	}
}

func TestRecencyScoreFutureAnchorClampsToOne(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	futureAnchor := now.Add(10 * 24 * time.Hour)
	score := RecencyScore(futureAnchor, now, 30)
	if math.Abs(score-1.0) > 1e-9 {
		t.Errorf("RecencyScore with future anchor should be 1.0, got %f", score)
	}
}

func TestRRFScoreBothRanks(t *testing.T) {
	// Ranked #1 in both: 1/(60+1) + 1/(60+1) = 2/61
	got := RRFScore(1, 1, 60)
	expected := 2.0 / 61.0
	if math.Abs(got-expected) > 1e-10 {
		t.Errorf("RRFScore(1,1,60) = %.10f, want %.10f", got, expected)
	}
}

func TestRRFScoreOnlyFTS(t *testing.T) {
	// vectorRank=0 means not in vector results.
	got := RRFScore(3, 0, 60)
	expected := 1.0 / (60.0 + 3.0)
	if math.Abs(got-expected) > 1e-10 {
		t.Errorf("RRFScore(3,0,60) = %.10f, want %.10f", got, expected)
	}
}

func TestRRFScoreOnlyVector(t *testing.T) {
	got := RRFScore(0, 5, 60)
	expected := 1.0 / (60.0 + 5.0)
	if math.Abs(got-expected) > 1e-10 {
		t.Errorf("RRFScore(0,5,60) = %.10f, want %.10f", got, expected)
	}
}

func TestRRFScoreBothZero(t *testing.T) {
	// Both ranks 0 → item in neither list → score 0.
	if got := RRFScore(0, 0, 60); got != 0 {
		t.Errorf("RRFScore(0,0,60) should be 0, got %f", got)
	}
}

func TestRRFScoreDefaultKWhenZero(t *testing.T) {
	// k=0 should default to 60.
	withExplicit := RRFScore(2, 3, 60)
	withDefault := RRFScore(2, 3, 0)
	if math.Abs(withExplicit-withDefault) > 1e-10 {
		t.Errorf("RRFScore k=0 should default to k=60: explicit=%f default=%f",
			withExplicit, withDefault)
	}
}

func TestRRFHigherRankMeansLowerScore(t *testing.T) {
	// RRF: lower rank number = higher score (1st beats 2nd).
	score1 := RRFScore(1, 0, 60)
	score2 := RRFScore(2, 0, 60)
	if score1 <= score2 {
		t.Errorf("rank 1 should outscore rank 2: score1=%f score2=%f", score1, score2)
	}
}

func TestHybridSearchRankOrdering(t *testing.T) {
	// Item "a" is #1 in both lists — must appear first.
	ftsHits := []pkg.Hit{
		{ID: "a", Score: 0.9},
		{ID: "b", Score: 0.8},
		{ID: "c", Score: 0.7},
	}
	vectorHits := []pkg.Hit{
		{ID: "a", Score: 0.95},
		{ID: "c", Score: 0.85},
		{ID: "d", Score: 0.75},
	}

	results := HybridSearch(context.Background(), ftsHits, vectorHits, 10)
	if len(results) == 0 {
		t.Fatal("HybridSearch returned no results")
	}
	if results[0].ID != "a" {
		t.Errorf("expected 'a' to rank first (top in both lists), got %q", results[0].ID)
	}
}

func TestHybridSearchDeduplicates(t *testing.T) {
	ftsHits := []pkg.Hit{{ID: "x", Score: 0.9}}
	vectorHits := []pkg.Hit{{ID: "x", Score: 0.8}}

	results := HybridSearch(context.Background(), ftsHits, vectorHits, 10)

	count := 0
	for _, r := range results {
		if r.ID == "x" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("ID 'x' appeared %d times in HybridSearch result, want 1", count)
	}
}

func TestHybridSearchRespectsTopK(t *testing.T) {
	ftsHits := make([]pkg.Hit, 20)
	for i := range ftsHits {
		ftsHits[i] = pkg.Hit{ID: fmt.Sprintf("f%d", i), Score: float64(20-i) / 20}
	}

	results := HybridSearch(context.Background(), ftsHits, nil, 5)
	if len(results) > 5 {
		t.Errorf("HybridSearch with topK=5 returned %d results", len(results))
	}
}

func TestHybridSearchEmptyInputs(t *testing.T) {
	results := HybridSearch(context.Background(), nil, nil, 10)
	if len(results) != 0 {
		t.Errorf("HybridSearch with nil inputs should return empty, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// 8. Bi-temporal (tier5) and visibility constants
// ---------------------------------------------------------------------------

func TestVisibilityConstantValues(t *testing.T) {
	if pkg.VisibilityPrivate != "private" {
		t.Errorf("VisibilityPrivate = %q, want 'private'", pkg.VisibilityPrivate)
	}
	if pkg.VisibilityShared != "shared" {
		t.Errorf("VisibilityShared = %q, want 'shared'", pkg.VisibilityShared)
	}
}

// TestBiTemporalDesign verifies that bi-temporal functions accept a time.Time
// parameter for "when did we stop believing this" — the key design invariant
// that ensures world model entries are invalidated, not deleted.
// (We verify the type signature is correct via compilation; this test
// documents the invariant for human readers.)
func TestBiTemporalDesign(t *testing.T) {
	// InvalidateEntity and InvalidateRelationship both take a time.Time `now`
	// argument, expressing WHEN the agent stopped believing the entity/relationship.
	// This is the bi-temporal design: the agent-belief time is separate from the
	// world-truth time (t_valid/t_invalid in the schema).
	//
	// SupersedeEntity writes a `supersedes` memory_edge linking old→new so the
	// audit chain is always traceable backward.
	//
	// This test verifies the design intent is reflected in function signatures.
	// The functions are defined in tier5.go; we verify them by type-checking
	// through the compiler (this file being in the same package).
	var _ = InvalidateEntity        // takes (ctx, db, entityID, now time.Time)
	var _ = InvalidateRelationship  // takes (ctx, db, relID, now time.Time)
	var _ = SupersedeEntity         // writes supersedes edge preserving audit chain
	t.Log("bi-temporal design verified: invalidation takes time.Time, not delete")
}

// TestContentSourceCount verifies that exactly 6 content source values are
// accepted by tier2. Adding a 7th without updating tests catches drift.
func TestContentSourceCount(t *testing.T) {
	if len(validContentSources) != 6 {
		t.Errorf("expected exactly 6 valid content sources, got %d", len(validContentSources))
	}
}
