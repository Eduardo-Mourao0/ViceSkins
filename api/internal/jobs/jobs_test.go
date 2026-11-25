package jobs

import (
	"context"
	"errors"
	"testing"
)

func TestRunAllIsolatesFailuresAndPanics(t *testing.T) {
	var ran []string
	list := []Job{
		{Name: "fails", Fn: func(context.Context) error { ran = append(ran, "fails"); return errors.New("boom") }},
		{Name: "panics", Fn: func(context.Context) error { ran = append(ran, "panics"); panic("oops") }},
		{Name: "ok", Fn: func(context.Context) error { ran = append(ran, "ok"); return nil }},
	}
	got := RunAll(context.Background(), list)

	if len(ran) != 3 {
		t.Fatalf("ran %v, want all 3 jobs", ran)
	}
	if got["fails"] == nil || got["panics"] == nil || got["ok"] != nil {
		t.Fatalf("results = %v", got)
	}
}

func TestRunAllSkipsWhenOutOfTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	got := RunAll(ctx, []Job{{Name: "late", Fn: func(context.Context) error { called = true; return nil }}})
	if called || !errors.Is(got["late"], context.Canceled) {
		t.Fatalf("called=%v result=%v; want skipped with context.Canceled", called, got["late"])
	}
}
