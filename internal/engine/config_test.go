package engine

import "testing"

func TestValidateConfigValue(t *testing.T) {
	ok := []struct{ key, val string }{
		{"ram", "512M"}, {"ram", "2G"}, {"ram", "16g"},
		{"storage", "5G"}, {"storage", "500M"},
		{"cpu", "1"}, {"cpu", "0.5"}, {"cpu", "2"},
		{"repo", ""}, {"repo", "https://github.com/you/app.git"},
		{"branch", ""}, {"branch", "main"}, {"branch", "release/1.2"},
		{"driver", "anything"}, // unknown key passes through
	}
	for _, c := range ok {
		if err := validateConfigValue(c.key, c.val); err != nil {
			t.Errorf("validateConfigValue(%q, %q) = %v, want nil", c.key, c.val, err)
		}
	}

	bad := []struct{ key, val string }{
		{"ram", "2GB"}, {"ram", "abc"}, {"ram", "-1"}, {"ram", "1.5G"}, {"ram", ""},
		{"storage", "lots"}, {"storage", "5"},
		{"cpu", "0"}, {"cpu", "-2"}, {"cpu", "abc"}, {"cpu", ""},
		{"repo", "ext::sh -c whoami"}, {"branch", "--upload-pack=evil"},
	}
	for _, c := range bad {
		if err := validateConfigValue(c.key, c.val); err == nil {
			t.Errorf("validateConfigValue(%q, %q) = nil, want error", c.key, c.val)
		}
	}
}
