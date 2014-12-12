package utils

import (
	"fmt"
	"strings"
)

type Retriable interface {
	Retry() bool
}

type DefaultRetriable struct {
	retry bool
}

func (dr DefaultRetriable) Retry() bool {
	return dr.retry
}

func NewRetriable(retry bool) Retriable {
	return DefaultRetriable{
		retry: retry,
	}
}

type RetriableError interface {
	Retriable
	error
}

type DefaultRetriableError struct {
	Retriable
	error
}

func NewRetriableError(retriable Retriable, err error) RetriableError {
	return &DefaultRetriableError{
		retriable,
		err,
	}
}

// Implements error
type MultiErr struct {
	errors []error
}

func (me MultiErr) Error() string {
	ret := make([]string, len(me.errors)+1)
	ret[0] = "Multiple error:"
	for ndx, err := range me.errors {
		ret[ndx+1] = fmt.Sprintf("\t%d: %s", ndx, err.Error())
	}
	return strings.Join(ret, "\n")
}

func NewMultiError(errs ...error) error {
	errors := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			errors = append(errors, err)
		}
	}
	return MultiErr{errors}
}
