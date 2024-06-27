package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/mdp/qrterminal/v3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type ErrCode string

const (
	EServerUnexpected ErrCode = "E000"
	EAlreadyConnected ErrCode = "E001"
)

var (
	ErrServerUnexpected = HTTPError{errors.New("server error occurred"), http.StatusInternalServerError, map[string]any{}, EServerUnexpected}
	ErrAlreadyConnected = HTTPError{errors.New("device already connected"), http.StatusBadRequest, map[string]any{}, EServerUnexpected}
)

var (
	ResponseErrUnexpected = &Response{
		Status:  http.StatusInternalServerError,
		Message: http.StatusText(http.StatusInternalServerError),
		Result:  nil,
		Error: &HTTPError{
			Status: http.StatusInternalServerError,
			Data:   nil,
			Code:   EServerUnexpected,
		},
	}
)

type Response struct {
	Status  int        `json:"status"`
	Message string     `json:"name"`
	Result  any        `json:"result"`
	Error   *HTTPError `json:"error"`
}

type Handler func(w http.ResponseWriter, r *http.Request) (*Response, error)

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			json.NewEncoder(w).Encode(ResponseErrUnexpected)
			return
		}
	}()

	w.Header().Add("Content-Type", "application/json")
	var handlerErr HTTPError

	res, err := h(w, r)
	if errors.As(err, &handlerErr) {
		w.WriteHeader(handlerErr.Status)
		resErr := &Response{
			Status:  handlerErr.Status,
			Message: handlerErr.Error(),
			Result:  nil,
			Error:   &handlerErr,
		}
		json.NewEncoder(w).Encode(resErr)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ResponseErrUnexpected)
		return
	}
	w.WriteHeader(res.Status)
	json.NewEncoder(w).Encode(res)
}

type WhatsappHandler struct {
	store *sqlstore.Container
}

func (h *WhatsappHandler) GenQR(w http.ResponseWriter, r *http.Request) (*Response, error) {
	var p struct {
		ClientDeviceID string `json:"client_device_id"`
	}
	err := json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		return nil, ErrServerUnexpected
	}

	var cli *whatsmeow.Client
	var deviceStore *store.Device

	cli = clients[p.ClientDeviceID]
	qr := qrs[p.ClientDeviceID]

	if cli == nil {
		deviceStore = h.store.NewDevice()
		cli = whatsmeow.NewClient(deviceStore, waLog.Stdout(p.ClientDeviceID, "INFO", true))
	}

	if qr != "" {
		return &Response{
			Status:  http.StatusOK,
			Message: "qr not scanned yet",
			Result:  map[string]string{"qr": qr},
			Error:   nil,
		}, nil
	}

	if cli.IsLoggedIn() {
		return nil, ErrAlreadyConnected
	}

	cli.AddEventHandler(eventHandler(p.ClientDeviceID))
	clients[p.ClientDeviceID] = cli

	ctx, cancel := context.WithCancel(context.Background())
	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		defer cancel()
		return nil, ErrAlreadyConnected
	}

	chImg := make(chan string)

	go func() {
		evt := <-qrChan
		if evt.Event == "code" {
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			go func() {
				time.Sleep(evt.Timeout - 10*time.Second)
				if !cli.IsLoggedIn() {
					fmt.Println("expiring qr code...")
					clients[p.ClientDeviceID] = nil
					qrs[p.ClientDeviceID] = ""
					cancel()
				}
			}()
			img, err := qrcode.Encode(evt.Code, qrcode.Medium, 512)
			if err != nil {
				cancel()
			}
			base64Img := base64.StdEncoding.EncodeToString(img)
			chImg <- "data:image/png;base64," + base64Img
		} else {
			fmt.Println("Login event:", evt.Event)
		}
	}()

	cli.Connect()

	qr = <-chImg
	qrs[p.ClientDeviceID] = qr
	return &Response{
		Status:  http.StatusOK,
		Message: "success create qr",
		Result:  map[string]string{"qr": qr},
		Error:   nil,
	}, nil
}

func (_ *WhatsappHandler) Logout(w http.ResponseWriter, r *http.Request) (*Response, error) {
	var p struct {
		DeviceID string `json:"client_device_id"`
	}
	err := json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		return nil, ErrServerUnexpected
	}

	cli := clients[p.DeviceID]
	cli.Logout()
	return &Response{
		Status:  http.StatusOK,
		Message: "logout success",
		Result:  map[string]any{"ok": true},
		Error:   nil,
	}, nil
}
