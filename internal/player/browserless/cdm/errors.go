package cdm

import "errors"

// ErrNoKey is the CDM reporting that the key for a sample is not available,
// which is what an expired license looks like.
var ErrNoKey = errors.New("decryption key not available")
