package repo

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexibleDurationJSON(t *testing.T) {
	t.Parallel()
	type wrap struct {
		D FlexibleDuration `json:"d"`
	}
	want := 750 * time.Millisecond
	b, err := json.Marshal(wrap{D: FlexibleDuration(want)})
	if err != nil {
		t.Fatal(err)
	}
	var got wrap
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if time.Duration(got.D) != want {
		t.Fatalf("got %v", time.Duration(got.D))
	}
}

func TestNewCrawlJobTimestampsUTC(t *testing.T) {
	t.Parallel()
	j := NewCrawlJob("id", CrawlParams{URL: "https://e.com", MaxDepth: 0, Workers: 1})
	if j.CreatedAt.Location() != time.UTC {
		t.Fatalf("location %v", j.CreatedAt.Location())
	}
}

func TestFlexibleDurationJSON_numberNanoseconds(t *testing.T) {
	t.Parallel()
	var d FlexibleDuration
	if err := json.Unmarshal([]byte(`500000000`), &d); err != nil {
		t.Fatal(err)
	}
	if time.Duration(d) != 500*time.Millisecond {
		t.Fatalf("got %v", time.Duration(d))
	}
}
