package task

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	maxTitleLen        = 200
	maxInstructions    = 50
	maxInstructionLen  = 10_000
	maxLabelLen        = 50
	maxCategoryNameLen = 100
	maxSlugLen         = 100
)

var slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// NormalizeTitle trims title and checks its length.
func NormalizeTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if n := utf8.RuneCountInString(title); n == 0 || n > maxTitleLen {
		return "", invalid("title", "must have between 1 and %d characters", maxTitleLen)
	}
	return title, nil
}

// NormalizeDescription trims the description, which must not be empty.
func NormalizeDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return "", invalid("description", "must not be empty")
	}
	return description, nil
}

// NormalizeInstructions trims each additional instruction, keeping their order.
func NormalizeInstructions(instructions []string) ([]string, error) {
	if len(instructions) > maxInstructions {
		return nil, invalid("additional_instructions", "must have at most %d items", maxInstructions)
	}
	out := make([]string, 0, len(instructions))
	for _, in := range instructions {
		in = strings.TrimSpace(in)
		if in == "" {
			return nil, invalid("additional_instructions", "items must not be empty")
		}
		if utf8.RuneCountInString(in) > maxInstructionLen {
			return nil, invalid("additional_instructions", "items must have at most %d characters", maxInstructionLen)
		}
		out = append(out, in)
	}
	return out, nil
}

// NormalizeLabel trims and lowercases a label name.
func NormalizeLabel(label string) (string, error) {
	label = strings.ToLower(strings.TrimSpace(label))
	if n := utf8.RuneCountInString(label); n == 0 || n > maxLabelLen {
		return "", invalid("labels", "names must have between 1 and %d characters", maxLabelLen)
	}
	return label, nil
}

// NormalizeLabels normalizes, deduplicates and sorts label names.
func NormalizeLabels(labels []string) ([]string, error) {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		l, err := NormalizeLabel(l)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// NormalizeCategoryName trims name and checks its length.
func NormalizeCategoryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n == 0 || n > maxCategoryNameLen {
		return "", invalid("name", "must have between 1 and %d characters", maxCategoryNameLen)
	}
	return name, nil
}

// ValidateSlug checks that slug has only lowercase letters, digits and hyphens.
func ValidateSlug(slug string) error {
	if len(slug) > maxSlugLen || !slugPattern.MatchString(slug) {
		return invalid("slug", "must have between 1 and %d lowercase letters, digits or hyphens", maxSlugLen)
	}
	return nil
}

// Slugify derives a slug from name: lowercase, accents removed, any other
// character turned into a hyphen, repeated hyphens collapsed and trimmed.
func Slugify(name string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// Combining mark left by the decomposition of an accented letter.
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			hyphen = false
		default:
			if !hyphen && b.Len() > 0 {
				b.WriteByte('-')
				hyphen = true
			}
		}
	}
	slug := strings.TrimSuffix(b.String(), "-")
	if len(slug) > maxSlugLen {
		slug = strings.TrimRight(slug[:maxSlugLen], "-")
	}
	return slug
}
