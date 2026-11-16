//go:build !scheduler.cores && !scheduler.workers

package runtime

import "internal/task"

type timerLock = task.PMutex
type synctestLock = task.PMutex
type channelLock = task.PMutex
