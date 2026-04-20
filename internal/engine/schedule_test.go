package engine

import "testing"

func TestSchedulerStartStop(t *testing.T) {
	s := NewScheduler()
	s.Start()
	s.Stop()
}
