package gate

import (
	"context"
	"testing"
	"time"
)

func TestDemoStoreScopesAndBoundsRuns(t *testing.T) {
	store := NewDemoStore(2, 2, time.Hour)
	ctx := context.Background()
	for _, run := range []Run{{ID: "one"}, {ID: "two"}, {ID: "three"}} {
		if err := store.Add(ctx, "visitor-a", run); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := store.List(ctx, "visitor-a")
	if err != nil || len(runs) != 2 || runs[0].ID != "three" || runs[1].ID != "two" {
		t.Fatalf("unexpected bounded history: %#v, %v", runs, err)
	}
	other, err := store.List(ctx, "visitor-b")
	if err != nil || len(other) != 0 {
		t.Fatalf("another visitor saw history: %#v, %v", other, err)
	}
}

func TestDemoStoreExpiresAndBoundsSessions(t *testing.T) {
	store := NewDemoStore(2, 2, time.Hour)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	ctx := context.Background()
	store.Add(ctx, "old", Run{ID: "old-run"})
	now = now.Add(30 * time.Minute)
	store.Add(ctx, "middle", Run{ID: "middle-run"})
	now = now.Add(31 * time.Minute)
	store.Add(ctx, "new", Run{ID: "new-run"})
	old, _ := store.List(ctx, "old")
	if len(old) != 0 {
		t.Fatal("expired session was retained")
	}
	if len(store.sessions) > 2 {
		t.Fatalf("session bound exceeded: %d", len(store.sessions))
	}
}
