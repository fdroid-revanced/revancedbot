package netx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lucasew/workspaced/pkg/driver"
	"github.com/lucasew/workspaced/pkg/driver/fetchurl"
	"github.com/lucasew/workspaced/pkg/logging"
)

// FetchURLs downloads the first successful URL into dest via the fetchurl driver
// (progress via httpclient). Optional hashAlgo/hash may be empty.
// Pass label via WithLabel(ctx, "download ReVanced CLI") for TUI task names.
func FetchURLs(ctx context.Context, urls []string, dest, hashAlgo, hash string) error {
	if len(urls) == 0 {
		return fmt.Errorf("no URLs")
	}
	if !logging.ContextHasLogger(ctx) {
		return fmt.Errorf("fetchurl requires logger on context")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	err = driver.With(ctx, func(d fetchurl.Driver) error {
		return d.Fetch(ctx, fetchurl.FetchOptions{
			URLs: urls,
			Algo: hashAlgo,
			Hash: hash,
			Out:  f,
		})
	})
	if err != nil {
		_ = os.Remove(dest)
		return err
	}
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() < 1024 {
		_ = os.Remove(dest)
		return fmt.Errorf("download too small (%d bytes)", st.Size())
	}
	return nil
}
