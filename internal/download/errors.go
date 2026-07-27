package download

import "errors"

// ErrBase is the wrap target for message-only failures with no deeper cause.
var ErrBase = errors.New("download error")
