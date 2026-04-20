package engine

import (
	"testing"
)

func TestContainerConfigDefaults(t *testing.T) {
	cfg := ContainerConfig{
		Name:  "test",
		Image: "nginx:alpine",
		Labels: map[string]string{
			"apod.site": "example.com",
		},
	}

	if cfg.Name != "test" {
		t.Errorf("got name %q, want test", cfg.Name)
	}
	if cfg.MemoryMB != 0 {
		t.Errorf("got memory %d, want 0", cfg.MemoryMB)
	}
	if cfg.CPUs != 0 {
		t.Errorf("got cpus %f, want 0", cfg.CPUs)
	}
}
