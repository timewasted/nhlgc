// Copyright 2014 Ryan Rogers. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nhlgc

import "fmt"

type LogicError struct {
	fnName, msg string
}

func newLogicError(fnName, msg string) LogicError {
	return LogicError{
		fnName: fnName,
		msg:    msg,
	}
}

func (e LogicError) Error() string {
	return e.fnName + ": " + e.msg
}

type NetworkError struct {
	LogicError
	StatusCode int
	Location   string
}

func newNetworkError(fnName, msg string, statusCode int, location string) NetworkError {
	return NetworkError{
		LogicError: newLogicError(fnName, msg),
		StatusCode: statusCode,
		Location:   location,
	}
}

func (e NetworkError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("%s: %s (status: %d - location: %s)", e.fnName, e.msg, e.StatusCode, e.Location)
	}
	return fmt.Sprintf("%s: %s (location: %s)", e.fnName, e.msg, e.Location)
}
