package imcs

import "errors"

var (
	ErrSearchComplete = errors.New("search is already complete")
	ErrNoActivePlan   = errors.New("cannot submit result without an active test plan")
	ErrTestInProgress = errors.New("a test is already in progress and must be completed first")
)
