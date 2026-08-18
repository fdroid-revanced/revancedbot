package download

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// verifyAdvertised checks Aptoide-stated size and md5 after a download.
// Empty constraints are skipped. md5 is the store digest, not a third-party proof.
func verifyAdvertised(path string, gotSize int64, cand aptoideVersion) error {
	if cand.Size > 0 && gotSize != cand.Size {
		return fmt.Errorf("size %d != advertised %d: %w", gotSize, cand.Size, ErrBase)
	}
	wantMD5 := strings.ToLower(strings.TrimSpace(cand.MD5))
	if wantMD5 == "" {
		return nil
	}
	sum, err := fileMD5(path)
	if err != nil {
		return err
	}
	if sum != wantMD5 {
		return fmt.Errorf("md5 %s != advertised %s: %w", sum, wantMD5, ErrBase)
	}
	return nil
}

func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
