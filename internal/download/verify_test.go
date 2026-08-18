package download

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyAdvertised(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.apk")
	body := []byte("hello-apk-body")
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(body)
	good := hex.EncodeToString(sum[:])

	if err := verifyAdvertised(p, int64(len(body)), aptoideVersion{MD5: good, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if err := verifyAdvertised(p, int64(len(body)), aptoideVersion{MD5: strings.ToUpper(good)}); err != nil {
		t.Fatal(err)
	}
	if err := verifyAdvertised(p, 3, aptoideVersion{Size: 4}); err == nil {
		t.Fatal("expected size mismatch")
	}
	if err := verifyAdvertised(p, int64(len(body)), aptoideVersion{MD5: "ffffffffffffffffffffffffffffffff", Size: int64(len(body))}); err == nil {
		t.Fatal("expected md5 mismatch")
	}
}
