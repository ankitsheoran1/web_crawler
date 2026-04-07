package repo

import (
	"fmt"
	"sync"
	"testing"
)

func TestStoreSaveGet(t *testing.T) {
	t.Parallel()
	s := NewStore()
	j := NewCrawlJob("job-1", CrawlParams{URL: "https://example.com", MaxDepth: 1, Workers: 1})
	s.Save(j)
	got, err := s.Get("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "job-1" || got.Status != JobPending {
		t.Fatalf("%+v", got)
	}
}

func TestStoreGetNotFound(t *testing.T) {
	t.Parallel()
	s := NewStore()
	_, err := s.Get("nope")
	if err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestStoreUpdate(t *testing.T) {
	t.Parallel()
	s := NewStore()
	s.Save(NewCrawlJob("x", CrawlParams{URL: "https://e.com", MaxDepth: 0, Workers: 1}))
	err := s.Update("x", func(j *Job) error {
		j.Status = JobCompleted
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	j, _ := s.Get("x")
	if j.Status != JobCompleted {
		t.Fatalf("status %s", j.Status)
	}
}

func TestStoreUpdateNotFound(t *testing.T) {
	t.Parallel()
	s := NewStore()
	err := s.Update("nope", func(j *Job) error { return nil })
	if err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestStoreList(t *testing.T) {
	t.Parallel()
	s := NewStore()
	s.Save(NewCrawlJob("a", CrawlParams{URL: "https://a.com", MaxDepth: 0, Workers: 1}))
	s.Save(NewCrawlJob("b", CrawlParams{URL: "https://b.com", MaxDepth: 0, Workers: 1}))
	if len(s.List()) != 2 {
		t.Fatalf("len %d", len(s.List()))
	}
}

func TestStoreConcurrent(t *testing.T) {
	t.Parallel()
	s := NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			s.Save(NewCrawlJob(id, CrawlParams{URL: "https://x.com", MaxDepth: 0, Workers: 1}))
			_, _ = s.Get(id)
			_ = s.Update(id, func(j *Job) error {
				j.Status = JobRunning
				return nil
			})
		}(fmt.Sprintf("job-%d", i))
	}
	wg.Wait()
}
