package memory

import (
	"context"
	"testing"
)

func TestPublisherStoresMessages(t *testing.T) {
	t.Parallel()

	pub := New()
	id1, err := pub.Publish(context.Background(), "topic-a", map[string]string{"k": "v"})
	if err != nil || id1 != "memory-1" {
		t.Fatalf("unexpected publish result id=%s err=%v", id1, err)
	}
	id2, err := pub.Publish(context.Background(), "topic-b", "payload")
	if err != nil || id2 != "memory-2" {
		t.Fatalf("unexpected publish result id=%s err=%v", id2, err)
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Topic != "topic-a" || msgs[1].Topic != "topic-b" {
		t.Fatalf("topics not recorded correctly: %+v", msgs)
	}

	msgs[0].Topic = "modified"
	if pub.Messages()[0].Topic == "modified" {
		t.Fatal("expected Messages() to return a copy")
	}
}
