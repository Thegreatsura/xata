package session

import "time"

// Keep fast polling for short branch creation and wakeup waits. For longer
// waits, double the interval every five seconds, up to one second. TCP retries
// after readiness continue to use the configured interval.
func statusPollInterval(initial, elapsed time.Duration) time.Duration {
	const stage = 5 * time.Second
	limit := max(initial, time.Second)
	interval := initial
	for steps := elapsed / stage; steps > 0 && interval < limit; steps-- {
		if interval >= limit/2 {
			return limit
		}
		interval *= 2
	}
	return interval
}
