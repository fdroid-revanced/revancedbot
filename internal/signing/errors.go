package signing

import "errors"

var (
	// ErrBase is the wrap target for message-only failures with no deeper cause.
	ErrBase = errors.New("signing error")
	// ErrPKCS12 is returned when a blob uses keystore_p12_b64 or storetype PKCS12.
	ErrPKCS12 = errors.New("PKCS12 signing blobs are not supported")
)
