//go:build scheduler.none

package main

func onFreshStack(fn func()) { fn() }

func yield() {}

func testBlockedFinalizer() {}

func testCleanupArguments() {}
