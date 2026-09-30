package main

import "testing"

func TestConformance(t *testing.T) {
	parentDone.Store(0)
	childDone.Store(0)
	resurrectedDone.Store(0)
	cleaned.Store(0)
	stopped.Store(0)
	for i := range interiorDone {
		interiorDone[i].Store(0)
	}
	interiorCleaned.Store(0)
	main()
}
