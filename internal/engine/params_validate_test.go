package engine

import (
	"testing"

	"github.com/aystro/apod/internal/models"
)

func TestValidateDriverParams(t *testing.T) {
	driver := &models.Driver{
		Parameters: map[string]models.DriverParam{
			"php_version": {Type: "string", Options: []string{"8.3", "8.4"}},
			"app_port":    {Type: "int", Default: "3000"},
			"free":        {Type: "string"},
		},
	}

	ok := []map[string]string{
		{"php_version": "8.4"},
		{"app_port": "8080"},
		{"free": "some.value-1_2/x"},
		{"undeclared": "anything; rm -rf /"}, // undeclared keys are ignored
		{},
	}
	for _, p := range ok {
		if err := validateDriverParams(driver, p); err != nil {
			t.Errorf("validateDriverParams(%v) = %v, want nil", p, err)
		}
	}

	bad := []map[string]string{
		{"app_port": "3000)}'; id > /pwned; echo '"}, // injection attempt
		{"app_port": "notanumber"},                   // wrong type
		{"php_version": "9.9"},                       // not in options
		{"free": "a b"},                              // whitespace
		{"free": "x\ny"},                             // newline
		{"free": "$(whoami)"},                        // shell metachars
	}
	for _, p := range bad {
		if err := validateDriverParams(driver, p); err == nil {
			t.Errorf("validateDriverParams(%v) = nil, want error", p)
		}
	}
}

func TestSetEnvValidation(t *testing.T) {
	e := newProcEngine(t)

	// Bad key shapes and newline-bearing values must be rejected before any
	// site lookup (so a missing site isn't what trips it).
	bad := []struct{ key, val string }{
		{"1BAD", "x"},
		{"has-dash", "x"},
		{"has space", "x"},
		{"OK", "line1\nINJECTED=evil"},
		{"OK\nX", "v"},
	}
	for _, c := range bad {
		if err := e.SetEnv(t.Context(), "x.example.com", c.key, c.val); err == nil {
			t.Errorf("SetEnv(%q=%q) = nil, want validation error", c.key, c.val)
		} else if ErrorKindOf(err) != KindInvalid {
			t.Errorf("SetEnv(%q): want Invalid, got %v", c.key, err)
		}
	}
}
