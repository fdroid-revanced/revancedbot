package version

import "testing"

func TestVersion_default(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must be non-empty")
	}
}
