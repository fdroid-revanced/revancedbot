package download

import (
	"strings"
	"testing"
)

func TestVersionSlug(t *testing.T) {
	if g, w := versionSlug("3.3.6"), "3-3-6"; g != w {
		t.Fatalf("got %q want %q", g, w)
	}
	if g, w := versionSlug("2026.29.0"), "2026-29-0"; g != w {
		t.Fatalf("got %q want %q", g, w)
	}
}

func TestScoreVariant_prefersUniversal(t *testing.T) {
	u := scoreVariant("universal android 10+ nodpi", "/apk/x/y/z-android-apk-download/")
	a := scoreVariant("arm64-v8a android 10+ 480dpi", "/apk/x/y/z-2-android-apk-download/")
	if !(u > a) {
		t.Fatalf("scores universal=%d abi=%d; want universal > abi", u, a)
	}
}

func TestIsBundleVariant(t *testing.T) {
	if !isBundleVariant("/apk/x/y/z-bundle-apk-download/", "something") {
		t.Fatal("path with bundle should be filtered")
	}
	if !isBundleVariant("/apk/x/y/z-android-apk-download/", "APKM FILE") {
		t.Fatal("window with apkm should be filtered")
	}
	if isBundleVariant("/apk/x/y/z-android-apk-download/", "universal android 10+ nodpi") {
		t.Fatal("plain universal APK must not be filtered")
	}
}

func TestReleaseHrefParsing(t *testing.T) {
	html := `
		<a href="/apk/bandcamp-inc/bandcamp/bandcamp-3-3-6-release/">x</a>
		<a href="/apk/bandcamp-inc/bandcamp/bandcamp-3-3-6-release/#disqus_thread">c</a>
		<a href="/apk/bandcamp-inc/bandcamp/bandcamp-3-3-5-release/">y</a>
	`
	links := uniqueInOrder(releaseHrefRe.FindAllStringSubmatch(html, -1))
	if len(links) != 2 {
		t.Fatalf("got %v", links)
	}
	if !strings.Contains(links[0], "3-3-6") {
		t.Fatalf("first should be 3-3-6: %v", links)
	}
}

func TestDownloadKeyAndPHPParsing(t *testing.T) {
	variantHTML := `href="/apk/bandcamp-inc/bandcamp/bandcamp-3-3-6-release/bandcamp-3-3-6-android-apk-download/download/?key=abc123def"`
	key := firstGroup(downloadKeyHrefRe, variantHTML)
	if key == "" || !strings.Contains(key, "key=abc123def") {
		t.Fatalf("key href: %q", key)
	}
	dlHTML := `please click <a id="download-link" rel="nofollow" href="/wp-content/themes/APKMirror/download.php?id=14153505&key=e58464b266c58357f65e6fc81b8b2da5667b358b">here</a>`
	php := firstGroup(downloadPHPRe, dlHTML)
	if php == "" || !strings.Contains(php, "download.php?id=") {
		t.Fatalf("php: %q", php)
	}
}
