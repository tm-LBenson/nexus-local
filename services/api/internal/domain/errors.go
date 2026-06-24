package domain

import "errors"

var (
	ErrInvalidEntity          = errors.New("invalid entity")
	ErrInvalidStateTransition = errors.New("invalid state transition")
)
