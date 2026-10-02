//go:build scheduler.none

package main

func fresh(fn func()) {
	fn()
}

func testPromotion() {}

func testCleanupArguments() {}
