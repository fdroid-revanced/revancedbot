package download

import (
	"archive/zip"
	"fmt"
	"os"
	"strings"
)

// MinAPKBytes is the smallest acceptable downloaded APK size.
// Smaller bodies are almost always error pages or truncated transfers.
const MinAPKBytes int64 = 1024

// ValidateAPK accepts a path only if it looks like a usable Android APK:
// non-trivial size, ZIP (PK) magic, not HTML, and contains AndroidManifest.xml.
// Callers should remove the file on rejection when it is a fresh download.
func ValidateAPK(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat apk: %w", err)
	}
	if st.Size() < MinAPKBytes {
		return fmt.Errorf("download too small (%d bytes), likely not an APK", st.Size())
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open apk: %w", err)
	}
	defer f.Close()

	head := make([]byte, 512)
	n, err := f.Read(head)
	if err != nil && n == 0 {
		return fmt.Errorf("read apk head: %w", err)
	}
	head = head[:n]

	if looksLikeHTML(head) {
		return fmt.Errorf("download looks like HTML, not an APK")
	}
	if n < 2 || head[0] != 'P' || head[1] != 'K' {
		snip := string(head)
		if len(snip) > 16 {
			snip = snip[:16]
		}
		return fmt.Errorf("download is not a ZIP/APK (magic %q)", snip)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("not a valid ZIP/APK: %w", err)
	}
	defer zr.Close()

	for _, zf := range zr.File {
		name := zf.Name
		if name == "AndroidManifest.xml" || strings.HasSuffix(name, "/AndroidManifest.xml") {
			return nil
		}
	}
	return fmt.Errorf("ZIP missing AndroidManifest.xml (not an APK)")
}

func looksLikeHTML(head []byte) bool {
	s := strings.TrimLeft(string(head), " \t\r\n")
	if len(s) < 5 {
		return false
	}
	low := strings.ToLower(s[:min(64, len(s))])
	return strings.HasPrefix(low, "<!doctype") ||
		strings.HasPrefix(low, "<html") ||
		strings.HasPrefix(low, "<head")
}
