package note

import (
	"strings"
	"testing"
)

func TestValidateNamespace(t *testing.T) {
	t.Parallel()

	valid := []string{"Yanki", "Yanki - Vault", "日本語", "a b c"}
	for _, namespace := range valid {
		if err := ValidateNamespace(namespace); err != nil {
			t.Errorf("ValidateNamespace(%q) = %v, want nil", namespace, err)
		}
	}

	invalid := []struct {
		name      string
		namespace string
		want      string
	}{
		{"empty", "   ", "Cannot be empty"},
		{"colon", "a:b", "Colon"},
		{"asterisk", "a*b", "Asterisk"},
		{"control", "a\nb", "Control"},
		{"too long", strings.Repeat("a", 61), "longer than 60"},
		{"zero width", "a\u200Bb", "Zero-width"},
	}

	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateNamespace(test.namespace)
			if err == nil {
				t.Fatalf("ValidateNamespace(%q) = nil, want an error", test.namespace)
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error %q does not mention %q", err.Error(), test.want)
			}
		})
	}
}

func TestSanitizeNamespace(t *testing.T) {
	t.Parallel()

	if got := SanitizeNamespace("  Yanki  "); got != "Yanki" {
		t.Errorf("SanitizeNamespace trimmed to %q, want %q", got, "Yanki")
	}

	// A decomposed "é" normalizes to a single code point.
	decomposed := "Cafe\u0301"
	if got := SanitizeNamespace(decomposed); got != "Café" {
		t.Errorf("SanitizeNamespace = %q, want %q", got, "Café")
	}
}
