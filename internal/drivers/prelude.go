// Package drivers registers the minimal workspaced driver set revancedbot needs.
// Blank-import this package from main (and tests that exercise network drivers).
package drivers

import (
	// Progress-aware HTTP (WithProgress on Transport).
	_ "github.com/lucasew/workspaced/pkg/driver/httpclient/native"
	// Known-URL downloads (uses httpclient under the hood).
	_ "github.com/lucasew/workspaced/pkg/driver/fetchurl/fetchurl"
)
