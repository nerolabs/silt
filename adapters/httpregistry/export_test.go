package httpregistry

import "time"

// SetPublishPollKnobsForTest shrinks the publish poll timing so the regression
// tests exercise the poll/re-submit loop in milliseconds. Returns a restore
// func; tests must defer it.
func SetPublishPollKnobsForTest(interval, timeout, resubmitEvery time.Duration) (restore func()) {
	oi, ot, or := publishPollInterval, publishPollTimeout, publishResubmitEvery
	publishPollInterval, publishPollTimeout, publishResubmitEvery = interval, timeout, resubmitEvery
	return func() {
		publishPollInterval, publishPollTimeout, publishResubmitEvery = oi, ot, or
	}
}
