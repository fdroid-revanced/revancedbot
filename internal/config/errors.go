package config

import "errors"

// ErrBase is the wrap target for message-only failures with no deeper cause.
var ErrBase = errors.New("config error")

// ErrMissingAuthorityDoc is returned when REPO/revancedbot.yaml is absent and --config is unset.
var ErrMissingAuthorityDoc = errors.New("missing authority doc")
