package response

import (
	"errors"
	"fmt"
	"net/http"
)

type ErrCode string

var EServerUnexpected ErrCode = "E000"
var ErrServerUnexpected = errors.New("server error occurred")

var (
	ResponseErrUnexpected = &Response{
		Status:  http.StatusInternalServerError,
		Message: http.StatusText(http.StatusInternalServerError),
		Result:  nil,
		Error: &ErrorResponse{
			Status: http.StatusInternalServerError,
			Data:   nil,
			Code:   EServerUnexpected,
		},
	}
)

type Response struct {
	Status  int            `json:"status"`
	Message string         `json:"name"`
	Result  any            `json:"result"`
	Error   *ErrorResponse `json:"error"`
}

type ErrorResponse struct {
	E      error
	Status int     `json:"status"`
	Data   any     `json:"data"`
	Code   ErrCode `json:"code"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("err: %s, with error code %s", e.E.Error(), e.Code)
}

func (e *ErrorResponse) Is(other error) bool {
	return errors.Is(other, e.E)
}
