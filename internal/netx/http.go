// Package netx wraps workspaced httpclient/fetchurl for revancedbot.
package netx

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/lucasew/workspaced/pkg/driver"
	"github.com/lucasew/workspaced/pkg/driver/httpclient"
	"github.com/lucasew/workspaced/pkg/logging"
)

// WithLabel sets a human-readable progress task name for HTTP work under ctx.
func WithLabel(ctx context.Context, label string) context.Context {
	return httpclient.WithTaskLabel(ctx, label)
}

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
