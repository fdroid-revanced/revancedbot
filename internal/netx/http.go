// Package netx wraps workspaced httpclient/fetchurl for revancedbot.
package netx

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"time"

	"workspaced/pkg/driver"
	"workspaced/pkg/driver/httpclient"
	"workspaced/pkg/logging"
)

// Client returns a progress-aware HTTP client (workspaced httpclient driver when
// registered; otherwise DefaultTransport + WithProgress).
func Client(ctx context.Context) *http.Client {
	return buildClient(ctx, false)
}

// ClientWithJar is Client plus a cookie jar (multi-step scrapers).
func ClientWithJar(ctx context.Context) *http.Client {
	return buildClient(ctx, true)
}

func buildClient(ctx context.Context, withJar bool) *http.Client {
	var base *http.Client
	if logging.ContextHasLogger(ctx) {
		if d, err := driver.Get[httpclient.Driver](ctx); err == nil && d != nil {
			base = d.Client()
		}
	}
	if base == nil {
		base = &http.Client{
			Transport: httpclient.WithProgress(http.DefaultTransport),
		}
	}
	// Clone so we can set Timeout/Jar without mutating the driver singleton.
	c := *base
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Minute
	}
	if withJar {
		jar, err := cookiejar.New(nil)
		if err == nil {
			c.Jar = jar
		}
	}
	return &c
}
