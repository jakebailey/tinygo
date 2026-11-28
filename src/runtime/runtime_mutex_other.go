//go:build !exec.multicore && !exec.workers

package runtime

import "internal/task"

type timerLock = task.PMutex
type synctestLock = task.PMutex
type channelLock = task.PMutex
