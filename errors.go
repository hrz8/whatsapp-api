package main

import (
	"errors"
	"fmt"
	"net/http"
)

type ErrCode string

const (
	EServerUnexpected ErrCode = "E000"
	EAlreadyConnected ErrCode = "E001"
)

var (
	ErrServerUnexpected = errors.New("server error occurred")
	ErrAlreadyConnected = errors.New("device already connected")
)

type ErrorResponse struct {
	error
	Status int     `json:"status"`
	Data   any     `json:"data"`
	Code   ErrCode `json:"code"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("err: %s, with error code %s", e.error.Error(), e.Code)
}

var (
	ErrResponseServerUnexpected = &ErrorResponse{
		ErrServerUnexpected,
		http.StatusInternalServerError,
		map[string]any{},
		EServerUnexpected,
	}
	ErrResponseAlreadyConnected = &ErrorResponse{
		ErrAlreadyConnected,
		http.StatusBadRequest,
		map[string]any{},
		EAlreadyConnected,
	}
)
