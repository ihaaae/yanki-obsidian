package note

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// forbiddenRune describes a character Anki (or Yanki) cannot store in a
// namespace, for error messages.
type forbiddenRune struct {
	r           rune
	description string
}

var forbiddenRunes = buildForbiddenRunes()

func buildForbiddenRunes() []forbiddenRune {
	runes := []forbiddenRune{{':', "Colon"}, {'*', "Asterisk"}}

	for r := rune(0x00); r <= 0x1F; r++ {
		runes = append(runes, forbiddenRune{r, "Control character"})
	}

	runes = append(runes, forbiddenRune{0x7F, "Delete"})

	for r := rune(0x80); r <= 0x9F; r++ {
		runes = append(runes, forbiddenRune{r, "Control character"})
	}

	runes = append(runes,
		forbiddenRune{0x00A0, "Non-breaking Space"},
		forbiddenRune{0x00AD, "Soft Hyphen"},
		forbiddenRune{0x200B, "Zero-width Space"},
		forbiddenRune{0x200C, "Zero-width Non-joiner"},
		forbiddenRune{0x200D, "Zero-width Joiner"},
		forbiddenRune{0x200E, "Left-to-right Mark"},
		forbiddenRune{0x200F, "Right-to-left Mark"},
		forbiddenRune{0x202A, "Left-to-right Embedding"},
		forbiddenRune{0x202B, "Right-to-left Embedding"},
		forbiddenRune{0x202C, "Pop Directional Formatting"},
		forbiddenRune{0x202D, "Left-to-right Override"},
		forbiddenRune{0x202E, "Right-to-left Override"},
		forbiddenRune{0xFEFF, "Byte Order Mark (BOM)"},
	)

	return runes
}

// SanitizeNamespace forgives leading and trailing whitespace and Unicode
// normalization differences. It does not remove forbidden characters; callers
// that create data should validate first.
func SanitizeNamespace(namespace string) string {
	return strings.TrimSpace(norm.NFC.String(namespace))
}

// ValidateNamespace reports whether a namespace is safe to store in Anki.
//
// Validation is strict rather than silently correcting, because the namespace
// is how Yanki recognizes the notes it manages: quietly changing it would hide
// notes from the user.
func ValidateNamespace(namespace string) error {
	var problems []string

	trimmed := strings.TrimSpace(namespace)
	if trimmed == "" {
		problems = append(problems, "Cannot be empty")
	}

	if utf8.RuneCountInString(trimmed) > NamespaceMaxLength {
		problems = append(problems, fmt.Sprintf("Cannot be longer than %d characters", NamespaceMaxLength))
	}

	for _, forbidden := range forbiddenRunes {
		if strings.ContainsRune(namespace, forbidden.r) {
			problems = append(problems, fmt.Sprintf("Forbidden character: %s: %q", forbidden.description, string(forbidden.r)))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid namespace %q:\n\t- %s", namespace, strings.Join(problems, "\n\t- "))
	}

	return nil
}

// ValidateAndSanitizeNamespace validates a namespace and returns its sanitized
// form.
func ValidateAndSanitizeNamespace(namespace string) (string, error) {
	if err := ValidateNamespace(namespace); err != nil {
		return "", err
	}

	return SanitizeNamespace(namespace), nil
}
