package revanced

import (
	"testing"
)

func TestParseListVersions(t *testing.T) {
	const sample = `
Package name: com.example.app
Most common compatible versions:
19.1.0
18.9.0
Package name: com.other.app
Most common compatible versions:
Any
`
	jobs := ParseListVersions(sample)
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs: %+v", len(jobs), jobs)
	}
	if jobs[0].PackageID != "com.example.app" || len(jobs[0].Versions) != 2 {
		t.Fatalf("job0: %+v", jobs[0])
	}
	if jobs[0].Versions[0] != "19.1.0" {
		t.Fatalf("order: %+v", jobs[0].Versions)
	}
	if jobs[1].PackageID != "com.other.app" || len(jobs[1].Versions) != 1 || jobs[1].Versions[0] != "" {
		t.Fatalf("job1: %+v", jobs[1])
	}
}
