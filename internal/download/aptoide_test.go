package download

import (
	"context"
	"os"
	"testing"
)

func TestLooksLikeBundleURL(t *testing.T) {
	if !looksLikeBundleURL("https://cdn.example/app.apkm") {
		t.Fatal("apkm")
	}
	if !looksLikeBundleURL("https://cdn.example/xapk/foo") {
		t.Fatal("xapk path")
	}
	if looksLikeBundleURL("https://pool.apk.aptoide.com/store/app.apk") {
		t.Fatal("plain apk must pass")
	}
}

func TestIsAllDigits(t *testing.T) {
	if !isAllDigits("220489") {
		t.Fatal("digits")
	}
	if isAllDigits("1.2.3") || isAllDigits("") {
		t.Fatal("non-digits")
	}
}

func TestAptoide_implementsDownloader(t *testing.T) {
	var _ Downloader = &Aptoide{}
}

// Optional live smoke (opt-in): REVANCEDBOT_NETWORK=1 go test ./internal/download -run AptoideLive
func TestAptoideLive_bandcamp(t *testing.T) {
	if os.Getenv("REVANCEDBOT_NETWORK") != "1" {
		t.Skip("set REVANCEDBOT_NETWORK=1 for live Aptoide smoke")
	}
	dir := t.TempDir()
	res, err := (&Aptoide{}).Fetch(context.Background(), Request{PackageID: "com.bandcamp.android"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPK(res.Path); err != nil {
		t.Fatal(err)
	}
	t.Logf("ok source=%s path=%s", res.SourceID, res.Path)
}
