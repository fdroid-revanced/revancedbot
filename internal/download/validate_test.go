package download

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAPK_rejectsTiny(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tiny.apk")
	if err := os.WriteFile(p, []byte("PK\x03\x04"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPK(p); err == nil {
		t.Fatal("expected error for tiny file")
	}
}

func TestValidateAPK_rejectsHTML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "html.apk")
	body := []byte("<!DOCTYPE html><html><body>not an apk</body></html>")
	for len(body) < int(MinAPKBytes)+10 {
		body = append(body, ' ')
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPK(p); err == nil {
		t.Fatal("expected error for HTML")
	}
}

func TestValidateAPK_rejectsZIPWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nomf.apk")
	if err := writeZipStored(p, map[string][]byte{
		"readme.txt": make([]byte, int(MinAPKBytes)+64),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPK(p); err == nil {
		t.Fatal("expected error for ZIP without AndroidManifest.xml")
	}
}

func TestValidateAPK_acceptsMinimalAPK(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ok.apk")
	if err := writeZipStored(p, map[string][]byte{
		"AndroidManifest.xml": make([]byte, int(MinAPKBytes)+64),
		"classes.dex":         []byte("dex"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPK(p); err != nil {
		t.Fatalf("expected accept: %v", err)
	}
}

func TestAcceptCached_removesBad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.apk")
	body := []byte("<html>nope</html>")
	for len(body) < int(MinAPKBytes)+1 {
		body = append(body, 'x')
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AcceptCached(p); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("expected file removed")
	}
}

func writeZipStored(path string, files map[string][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		w, err := zw.CreateHeader(h)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := w.Write(content); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}
