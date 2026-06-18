package cli

import "testing"

func TestIsLoopbackListen(t *testing.T) {
	loopback := []string{"127.0.0.1:8443", "[::1]:8443", "localhost:8443", "127.0.0.1"}
	for _, a := range loopback {
		if !isLoopbackListen(a) {
			t.Errorf("isLoopbackListen(%q) = false, want true", a)
		}
	}
	public := []string{"0.0.0.0:8443", ":8443", "10.0.0.5:8443", "example.com:8443"}
	for _, a := range public {
		if isLoopbackListen(a) {
			t.Errorf("isLoopbackListen(%q) = true, want false", a)
		}
	}
}
