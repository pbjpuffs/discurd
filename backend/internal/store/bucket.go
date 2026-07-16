package store

import "time"

// BucketSizeMillis is the message-bucket width: 10 days, per the Discord
// bucketing pattern (docs/ARCHITECTURE.md §4).
const BucketSizeMillis int64 = 864_000_000

// BucketFromMillis maps a unix-milliseconds timestamp to its bucket.
func BucketFromMillis(ms int64) int {
	return int(ms / BucketSizeMillis)
}

// BucketFromTime maps a time to its bucket.
func BucketFromTime(t time.Time) int {
	return BucketFromMillis(t.UnixMilli())
}
