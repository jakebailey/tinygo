package main

import "testing"

func TestConformance(t *testing.T) {
	parentDone.Store(0)
	childDone.Store(0)
	resurrectedDone.Store(0)
	cleaned.Store(0)
	stopped.Store(0)
	main()
}
