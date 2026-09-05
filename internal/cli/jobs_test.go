package cli

import (
	"testing"

	"github.com/lucasew/revancedbot/internal/revanced"
)

func TestFormatJobs(t *testing.T) {
	jobs := []revanced.Job{
		{PackageID: "com.example.app", Versions: []string{"19.1.0", "18.9.0"}},
		{PackageID: "com.other.app", Versions: []string{""}},
		{PackageID: "com.empty.app"},
	}
	got := formatJobs(jobs)
	want := []string{
		"com.example.app\t19.1.0,18.9.0",
		"com.other.app\tAny",
		"com.empty.app\t",
	}
	if len(got) != len(want) {
		t.Fatalf("formatJobs() len=%d; want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("formatJobs()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
