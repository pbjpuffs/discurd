package store

import (
	"testing"
	"time"
)

func TestBucketFromMillisBoundaries(t *testing.T) {
	cases := []struct {
		name string
		ms   int64
		want int
	}{
		{"epoch", 0, 0},
		{"last ms of bucket 0", BucketSizeMillis - 1, 0},
		{"first ms of bucket 1", BucketSizeMillis, 1},
		{"last ms of bucket 1", 2*BucketSizeMillis - 1, 1},
		{"first ms of bucket 2", 2 * BucketSizeMillis, 2},
		{"mid bucket", 5*BucketSizeMillis + 12345, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BucketFromMillis(tc.ms); got != tc.want {
				t.Fatalf("BucketFromMillis(%d) = %d, want %d", tc.ms, got, tc.want)
			}
		})
	}
}

func TestBucketSizeIsTenDays(t *testing.T) {
	if want := int64(10 * 24 * time.Hour / time.Millisecond); BucketSizeMillis != want {
		t.Fatalf("BucketSizeMillis = %d, want %d (10 days)", BucketSizeMillis, want)
	}
}

func TestBucketFromTimeMatchesMillis(t *testing.T) {
	// 2026-07-16T00:00:00Z is a stable reference date.
	ref := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	if got, want := BucketFromTime(ref), BucketFromMillis(ref.UnixMilli()); got != want {
		t.Fatalf("BucketFromTime = %d, want %d", got, want)
	}
	// Known value: 1784160000000 ms / 864000000 = 2065 (exact boundary).
	if got := BucketFromTime(ref); got != 2065 {
		t.Fatalf("BucketFromTime(2026-07-16) = %d, want 2065", got)
	}
}

func TestAdjacentTimesStraddlingBoundary(t *testing.T) {
	// One millisecond apart, but in different buckets.
	boundary := 100 * BucketSizeMillis
	before := time.UnixMilli(boundary - 1)
	after := time.UnixMilli(boundary)
	if b1, b2 := BucketFromTime(before), BucketFromTime(after); b1 != 99 || b2 != 100 {
		t.Fatalf("straddle: got %d and %d, want 99 and 100", b1, b2)
	}
}
