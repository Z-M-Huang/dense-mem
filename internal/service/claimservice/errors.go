package claimservice

import "errors"

// ErrSupportingFragmentMissing is returned when one or more of the requested
// supporting SourceFragment nodes cannot be found for the given profile, or
// when a matching fragment has been retracted.
//
// Callers may wrap this error: errors.Is(err, ErrSupportingFragmentMissing).
var ErrSupportingFragmentMissing = errors.New("supporting fragment missing or retracted")

// ErrClaimNotFound is returned when a claim cannot be found for the given
// profile and claim ID.
//
// Callers may wrap this error: errors.Is(err, ErrClaimNotFound).
var ErrClaimNotFound = errors.New("claim not found")
