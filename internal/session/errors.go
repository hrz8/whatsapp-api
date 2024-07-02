package session

import (
	"errors"
	"net/http"

	"github.com/hrz8/whatsapp-api/pkg/response"
)

const (
	EAlreadyConnected  response.ErrCode = "E001"
	ENotLogin          response.ErrCode = "E002"
	ERecipientNotFound response.ErrCode = "E003"
)

var (
	ErrAlreadyConnected  = errors.New("device already connected")
	ErrNotLogin          = errors.New("device not login yet")
	ErrRecipientNotFound = errors.New("recipient number not found")
)

var (
	ErrRespServerUnexpected = &response.ErrorResponse{
		E:      response.ErrServerUnexpected,
		Status: http.StatusInternalServerError,
		Data:   map[string]any{},
		Code:   response.EServerUnexpected,
	}
	ErrRespAlreadyConnected = &response.ErrorResponse{
		E:      ErrAlreadyConnected,
		Status: http.StatusBadRequest,
		Data:   map[string]any{},
		Code:   EAlreadyConnected,
	}
	ErrRespNotLogin = &response.ErrorResponse{
		E:      ErrNotLogin,
		Status: http.StatusBadRequest,
		Data:   map[string]any{},
		Code:   ENotLogin,
	}
	ErrRespRecipientNotFound = &response.ErrorResponse{
		E:      ErrRecipientNotFound,
		Status: http.StatusBadRequest,
		Data:   map[string]any{},
		Code:   ERecipientNotFound,
	}
)
