package engine

import "testing"

func TestCronManagerStartStop(t *testing.T) {
	cm := NewCronManager()
	cm.Start()
	cm.Stop()
}
