package main

import "fmt"

type HTTPError struct {
	error
	Status int     `json:"status"`
	Data   any     `json:"data"`
	Code   ErrCode `json:"code"`
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("err: %s, with error code %s", e.error.Error(), e.Code)
}
