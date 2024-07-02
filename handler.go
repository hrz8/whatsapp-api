package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hrz8/whatsapp-api/pkg/response"
)

type Handler func(w http.ResponseWriter, r *http.Request) (*response.Response, error)

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			json.NewEncoder(w).Encode(response.ResponseErrUnexpected)
			return
		}
	}()

	w.Header().Add("Content-Type", "application/json")
	var handlerErr *response.ErrorResponse

	res, err := h(w, r)
	if errors.As(err, &handlerErr) {
		w.WriteHeader(handlerErr.Status)
		resErr := &response.Response{
			Status:  handlerErr.Status,
			Message: handlerErr.Error(),
			Result:  nil,
			Error:   handlerErr,
		}
		json.NewEncoder(w).Encode(resErr)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response.ResponseErrUnexpected)
		return
	}
	w.WriteHeader(res.Status)
	json.NewEncoder(w).Encode(res)
}
