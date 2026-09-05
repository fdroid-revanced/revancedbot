package download

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/lucasew/workspaced/pkg/logging"
)

// connectCDP attaches to an operator CDP URL. Tests may stub it.
var connectCDP = connectBrowser

// connectBrowser is rod.New().ControlURL(cdp).Connect() only (INV-06).
// An empty CDP URL does not call Connect — rod would launch Chrome.
func connectBrowser(ctx context.Context, cdp string) (*rod.Browser, error) {
	if strings.TrimSpace(cdp) == "" {
		return nil, ErrNeedBrowser
	}
	browser := rod.New().Context(ctx).ControlURL(cdp)
	if err := browser.Connect(); err != nil {
		// Do not Close: a failed Connect leaves no CDP client (Close panics).
		return nil, fmt.Errorf("cdp connect: %w", err)
	}
	return browser, nil
}

func browserGet(ctx context.Context, cdp, pageURL string) ([]byte, error) {
	if logging.ContextHasLogger(ctx) {
		logging.GetLogger(ctx).Info("browser upgrade", "url", pageURL)
	}
	browser, err := connectCDP(ctx, cdp)
	if err != nil {
		return nil, err
	}
	defer browser.Close()

	page, err := browser.Page(proto.TargetCreateTarget{URL: pageURL})
	if err != nil {
		return nil, fmt.Errorf("browser page: %w", err)
	}
	defer page.Close()
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("browser load: %w", err)
	}
	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("browser html: %w", err)
	}
	return []byte(html), nil
}

// rawHTTP is a completed HTTP response that may upgrade to rod.
type rawHTTP struct {
	URL    string
	Status int
	Body   []byte
	HTMLOK bool // scrapers accept 200 HTML; JSON/API callers do not
}

func (r rawHTTP) finish(ctx context.Context, cdp string) ([]byte, error) {
	challenge := looksLikeChallenge(string(r.Body))
	html := looksLikeHTML(r.Body)
	usable := r.Status == http.StatusOK && !challenge && (r.HTMLOK || !html)
	if usable {
		return r.Body, nil
	}
	if r.Status == http.StatusOK || httpStatusNeedsBrowser(r.Status) {
		return browserGet(ctx, cdp, r.URL)
	}
	return nil, fmt.Errorf("HTTP %d for %s: %w", r.Status, r.URL, ErrBase)
}

func httpStatusNeedsBrowser(status int) bool {
	switch status {
	case http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	default:
		return false
	}
}

func looksLikeChallenge(body string) bool {
	low := strings.ToLower(body)
	for _, mark := range []string{
		"just a moment",
		"cf-browser-verification",
		"checking your browser",
		"attention required",
		"/cdn-cgi/challenge-platform",
		"cf-challenge",
	} {
		if strings.Contains(low, mark) {
			return true
		}
	}
	return false
}
