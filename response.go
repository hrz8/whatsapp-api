package main

import "net/http"

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
