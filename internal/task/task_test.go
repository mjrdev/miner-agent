package task

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestStatusTransitions(t *testing.T) {
	allowed := map[[2]Status]bool{
		{StatusDraft, StatusReady}:          true,
		{StatusDraft, StatusCancelled}:      true,
		{StatusReady, StatusInProgress}:     true,
		{StatusReady, StatusDraft}:          true,
		{StatusReady, StatusCancelled}:      true,
		{StatusInProgress, StatusInReview}:  true,
		{StatusInProgress, StatusFailed}:    true,
		{StatusInProgress, StatusReady}:     true,
		{StatusInProgress, StatusCancelled}: true,
		{StatusInReview, StatusDone}:        true,
		{StatusInReview, StatusReady}:       true,
		{StatusInReview, StatusCancelled}:   true,
		{StatusFailed, StatusReady}:         true,
		{StatusFailed, StatusCancelled}:     true,
	}
	all := []Status{StatusDraft, StatusReady, StatusInProgress, StatusInReview, StatusDone, StatusFailed, StatusCancelled}

	for _, from := range all {
		if !from.Valid() {
			t.Errorf("%s.Valid() = false", from)
		}
		for _, to := range all {
			want := allowed[[2]Status{from, to}]
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s -> %s = %v, want %v", from, to, got, want)
			}
		}
	}

	if Status("archived").Valid() {
		t.Error(`Status("archived").Valid() = true`)
	}
}

func TestNormalizeLabels(t *testing.T) {
	got, err := NormalizeLabels([]string{"Backend", " backend ", "API"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"api", "backend"}; !slices.Equal(got, want) {
		t.Errorf("NormalizeLabels() = %v, want %v", got, want)
	}

	for _, bad := range []string{"  ", strings.Repeat("x", 51)} {
		if _, err := NormalizeLabels([]string{bad}); !isValidation(err, "labels") {
			t.Errorf("NormalizeLabels(%q) error = %v, want validation error", bad, err)
		}
	}
}

func TestNormalizeInstructions(t *testing.T) {
	got, err := NormalizeInstructions([]string{"passo 2", " passo 1 "})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"passo 2", "passo 1"}; !slices.Equal(got, want) {
		t.Errorf("NormalizeInstructions() = %v, want %v", got, want)
	}

	tooMany := make([]string, 51)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	for name, in := range map[string][]string{
		"blank item": {"   "},
		"too long":   {strings.Repeat("x", 10_001)},
		"too many":   tooMany,
	} {
		if _, err := NormalizeInstructions(in); !isValidation(err, "additional_instructions") {
			t.Errorf("%s: error = %v, want validation error", name, err)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	if got, err := NormalizeTitle("  Corrigir login "); err != nil || got != "Corrigir login" {
		t.Errorf("NormalizeTitle() = %q, %v", got, err)
	}
	if _, err := NormalizeTitle(strings.Repeat("é", 200)); err != nil {
		t.Errorf("200 characters rejected: %v", err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("x", 201)} {
		if _, err := NormalizeTitle(bad); !isValidation(err, "title") {
			t.Errorf("NormalizeTitle(%q) error = %v, want validation error", bad, err)
		}
	}
}

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"Integrações Back-end": "integracoes-back-end",
		"  Infra  ":            "infra",
		"API & Web / v2":       "api-web-v2",
		"!!!":                  "",
	}
	for in, want := range tests {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Slugify(strings.Repeat("ab ", 60)); len(got) > 100 || strings.HasSuffix(got, "-") {
		t.Errorf("long slug not trimmed: %q", got)
	}
}

func TestValidateSlug(t *testing.T) {
	for _, ok := range []string{"backend", "back-end-2"} {
		if err := ValidateSlug(ok); err != nil {
			t.Errorf("ValidateSlug(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Com Espaço", "UPPER", strings.Repeat("a", 101)} {
		if err := ValidateSlug(bad); !isValidation(err, "slug") {
			t.Errorf("ValidateSlug(%q) = %v, want validation error", bad, err)
		}
	}
}

func isValidation(err error, field string) bool {
	var ve *ValidationError
	return errors.As(err, &ve) && ve.Field == field
}

func TestValidateRepoURL(t *testing.T) {
	for _, ok := range []string{"https://github.com/org/repo", "https://gitlab.example.com/group/sub/repo.git"} {
		if err := ValidateRepoURL(ok); err != nil {
			t.Errorf("ValidateRepoURL(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{
		"",
		"https://user:token@github.com/org/repo",
		"https://user@github.com/org/repo",
		"git@github.com:org/repo.git",
		"http://github.com/org/repo",
		"https://github.com",
		"https://github.com/",
		"https:///org/repo",
		"github.com/org/repo",
		"https://github.com/" + strings.Repeat("a", 500),
	} {
		if err := ValidateRepoURL(bad); !isValidation(err, "repo_url") {
			t.Errorf("ValidateRepoURL(%q) = %v, want validation error", bad, err)
		}
	}
}

func TestValidateBaseBranch(t *testing.T) {
	for _, ok := range []string{"main", "release/1.x", "feature/ABC-1_x", strings.Repeat("a", 255)} {
		if err := ValidateBaseBranch(ok); err != nil {
			t.Errorf("ValidateBaseBranch(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "feature..x", "-x", "/x", "x/", "com espaço", "x~1", strings.Repeat("a", 256)} {
		if err := ValidateBaseBranch(bad); !isValidation(err, "base_branch") {
			t.Errorf("ValidateBaseBranch(%q) = %v, want validation error", bad, err)
		}
	}
}
