package memory

import (
	"context"
	"testing"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

func TestJobStoreLifecycle(t *testing.T) {
	t.Parallel()

	store := NewJobStore()
	ctx := context.Background()
	job := crawler.Job{ID: "job-1", Status: crawler.JobStatusQueued}

	if err := store.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if err := store.CreateJob(ctx, job); err == nil {
		t.Fatal("expected duplicate job error")
	}
	if err := store.UpdateJobStatus(ctx, job.ID, crawler.JobStatusRunning, "", crawler.JobCounters{}); err != nil {
		t.Fatalf("UpdateJobStatus running error = %v", err)
	}
	record := crawler.PageRecord{JobID: job.ID, URL: "https://example.com"}
	if err := store.RecordPage(ctx, record); err != nil {
		t.Fatalf("RecordPage() error = %v", err)
	}
	pages, err := store.ListPages(ctx, job.ID)
	if err != nil || len(pages) != 1 {
		t.Fatalf("ListPages() unexpected result: pages=%v err=%v", pages, err)
	}
	pages[0].URL = "modified"
	if store.pages[job.ID][0].URL != "https://example.com" {
		t.Fatal("expected ListPages to return a copy")
	}

	err = store.UpdateJobStatus(
		ctx,
		job.ID,
		crawler.JobStatusSucceeded,
		"done",
		crawler.JobCounters{PagesSucceeded: 1},
	)
	if err != nil {
		t.Fatalf("UpdateJobStatus succeeded error = %v", err)
	}
	final, err := store.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if final.Status != crawler.JobStatusSucceeded || final.Started == nil || final.Finished == nil {
		t.Fatalf("expected timestamps set, got %+v", final)
	}
	if final.ErrorText != "done" || final.Counters.PagesSucceeded != 1 {
		t.Fatalf("expected counters/error text to persist, got %+v", final)
	}
}
