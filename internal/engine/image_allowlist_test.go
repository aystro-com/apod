package engine

import "testing"

func TestImageAllowed(t *testing.T) {
	// With no allowlist configured, everything is allowed (default behavior).
	saved := allowedImagePrefixes
	t.Cleanup(func() { allowedImagePrefixes = saved })

	allowedImagePrefixes = nil
	for _, img := range []string{"nginx", "evil.io/x/miner:latest", ""} {
		if !imageAllowed(img) {
			t.Errorf("no allowlist: imageAllowed(%q) = false, want true", img)
		}
	}

	allowedImagePrefixes = []string{"docker.io/library/", "ghcr.io/aystro-com/"}
	allowed := []string{"docker.io/library/nginx:1", "ghcr.io/aystro-com/apod-ui:latest"}
	for _, img := range allowed {
		if !imageAllowed(img) {
			t.Errorf("imageAllowed(%q) = false, want true", img)
		}
	}
	blocked := []string{"nginx", "evil.io/x/miner", "ghcr.io/someoneelse/x"}
	for _, img := range blocked {
		if imageAllowed(img) {
			t.Errorf("imageAllowed(%q) = true, want false", img)
		}
	}
}
