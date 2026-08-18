package download

import "testing"

func TestCanonicalPackage(t *testing.T) {
	t.Parallel()
	if got := CanonicalPackage("com.youtube.android"); got != "com.google.android.youtube" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalPackage("com.google.android.youtube"); got != "com.google.android.youtube" {
		t.Fatalf("passthrough %q", got)
	}
	if got := CanonicalPackage("com.youtube.music"); got != "com.google.android.apps.youtube.music" {
		t.Fatalf("music %q", got)
	}
}
