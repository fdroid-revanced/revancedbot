package app

import "errors"

var (
	// ErrBase is the wrap target for message-only failures with no deeper cause.
	ErrBase = errors.New("app error")
	// errStopWalk ends the version walk after a version has already staged (INV-05).
	errStopWalk = errors.New("do not try another version")
)
