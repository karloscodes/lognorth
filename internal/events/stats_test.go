package events

import (
	"testing"
	"time"
)

func TestStatBatch(t *testing.T) {
	db := newTestDB(t)
	bucket := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	next := bucket.Add(5 * time.Minute)
	existing := EndpointStat{APIKeyID: 1, Path: "/a", Bucket: bucket, Total: 2, AvgDuration: 100}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	batch := NewStatBatch()
	add := func(path string, isError bool, durationMS int, at time.Time) {
		t.Helper()
		if err := batch.Add(db, 1, path, isError, durationMS, at); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	add("/a", false, 200, bucket.Add(time.Minute))  // avg (100*2+200)/3 = 133
	add("/a", true, 0, bucket.Add(2*time.Minute))   // counted, avg unchanged
	add("/a", false, 10, bucket.Add(3*time.Minute)) // avg (133*4+10)/5 = 108
	add("/b", true, 0, bucket)                      // new row, avg 0
	add("/b", false, 50, bucket)                    // avg (0*1+50)/2 = 25
	add("/a", false, 7, next)                       // next bucket, new row
	add("", false, 99, bucket)                      // no path: ignored
	err := batch.Flush(db)

	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	var rows []EndpointStat
	db.Order("path, bucket").Find(&rows)
	want := []EndpointStat{
		{Path: "/a", Bucket: bucket, Total: 5, Errors: 1, AvgDuration: 108},
		{Path: "/a", Bucket: next, Total: 1, Errors: 0, AvgDuration: 7},
		{Path: "/b", Bucket: bucket, Total: 2, Errors: 1, AvgDuration: 25},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		r := rows[i]
		if r.Path != w.Path || !r.Bucket.Equal(w.Bucket) || r.Total != w.Total || r.Errors != w.Errors || r.AvgDuration != w.AvgDuration {
			t.Errorf("row %d = %s %v total=%d errors=%d avg=%d, want %s %v total=%d errors=%d avg=%d",
				i, r.Path, r.Bucket, r.Total, r.Errors, r.AvgDuration, w.Path, w.Bucket, w.Total, w.Errors, w.AvgDuration)
		}
	}
	if rows[0].ID != existing.ID {
		t.Errorf("the existing row got a new ID %d, want %d (updated in place)", rows[0].ID, existing.ID)
	}
}
