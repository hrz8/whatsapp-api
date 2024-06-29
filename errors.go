package main

import (
	"errors"
	"fmt"
	"net/http"
)

type ErrCode string

const (
	EServerUnexpected  ErrCode = "E000"
	EAlreadyConnected  ErrCode = "E001"
	ENotLogin          ErrCode = "E002"
	ERecipientNotFound ErrCode = "E003"
)

var (
	ErrServerUnexpected  = errors.New("server error occurred")
	ErrAlreadyConnected  = errors.New("device already connected")
	ErrNotLogin          = errors.New("device not login yet")
	ErrRecipientNotFound = errors.New("recipient number not found")
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

func (e *ErrorResponse) Is(other error) bool {
	return errors.Is(other, e.error)
}

var (
	ErrRespServerUnexpected = &ErrorResponse{
		ErrServerUnexpected,
		http.StatusInternalServerError,
		map[string]any{},
		EServerUnexpected,
	}
	ErrRespAlreadyConnected = &ErrorResponse{
		ErrAlreadyConnected,
		http.StatusBadRequest,
		map[string]any{},
		EAlreadyConnected,
	}
	ErrRespNotLogin = &ErrorResponse{
		ErrNotLogin,
		http.StatusBadRequest,
		map[string]any{},
		ENotLogin,
	}
	ErrRespRecipientNotFound = &ErrorResponse{
		ErrRecipientNotFound,
		http.StatusBadRequest,
		map[string]any{},
		ERecipientNotFound,
	}
)
