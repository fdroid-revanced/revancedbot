package download

import "errors"

// ErrBase is the wrap target for message-only failures with no deeper cause.
var ErrBase = errors.New("download error")

// ErrNeedBrowser is returned when a page needs a browser and no CDP URL is set.
var ErrNeedBrowser = errors.New("needs browser, CDP absent")
