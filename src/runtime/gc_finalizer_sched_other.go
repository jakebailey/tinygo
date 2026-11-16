//go:build (gc.conservative || gc.precise || gc.boehm) && !scheduler.none && !exec.single && !exec.asyncify

package runtime

// spawnFinalizerRunner is the fallback for noncooperative schedulers.
// These schedulers run finalizers but do not install the idle GC hook.
func spawnFinalizerRunner() { go finalizerRunner() }

func initFinalizerScheduler() {}
