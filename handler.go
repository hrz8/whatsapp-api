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
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type Handler func(w http.ResponseWriter, r *http.Request) (*Response, error)

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			json.NewEncoder(w).Encode(ResponseErrUnexpected)
			return
		}
	}()

	w.Header().Add("Content-Type", "application/json")
	var handlerErr *ErrorResponse

	res, err := h(w, r)
	if errors.As(err, &handlerErr) {
		w.WriteHeader(handlerErr.Status)
		resErr := &Response{
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
		json.NewEncoder(w).Encode(ResponseErrUnexpected)
		return
	}
	w.WriteHeader(res.Status)
	json.NewEncoder(w).Encode(res)
}

type WhatsappHandler struct {
	store *sqlstore.Container
}

type ClientPayload struct {
	ClientDeviceID string `json:"client_device_id"`
}

func (h *WhatsappHandler) GenQR(w http.ResponseWriter, r *http.Request) (resp *Response, err error) {
	var p ClientPayload
	err = json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		return nil, ErrRespServerUnexpected
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
		resp = &Response{
			Status:  http.StatusOK,
			Message: "qr not scanned yet",
			Result:  map[string]string{"qr": qr},
			Error:   nil,
		}
		return
	}

	if cli.IsLoggedIn() {
		return nil, ErrRespAlreadyConnected
	}

	cli.AddEventHandler(eventHandler(p.ClientDeviceID))
	clients[p.ClientDeviceID] = cli

	ctx, cancel := context.WithCancel(context.Background())
	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		defer cancel()
		return nil, ErrRespAlreadyConnected
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
	resp = &Response{
		Status:  http.StatusOK,
		Message: "success create qr",
		Result:  map[string]string{"qr": qr},
		Error:   nil,
	}
	return
}

func (_ *WhatsappHandler) Logout(w http.ResponseWriter, r *http.Request) (resp *Response, err error) {
	var p ClientPayload
	err = json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		return nil, ErrRespServerUnexpected
	}

	cli := clients[p.ClientDeviceID]
	if !cli.IsLoggedIn() {
		return nil, ErrRespNotLogin
	}

	cli.Logout()

	resp = &Response{
		Status:  http.StatusOK,
		Message: "logout success",
		Result:  map[string]any{"ok": true},
		Error:   nil,
	}
	return
}

type SendMessagePayload struct {
	ClientDeviceID string `json:"client_device_id"`
	Recipient      string `json:"recipient"`
	Message        string `json:"message"`
}

func (_ *WhatsappHandler) SendMessage(w http.ResponseWriter, r *http.Request) (resp *Response, err error) {
	var p SendMessagePayload
	err = json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		return nil, ErrRespServerUnexpected
	}

	cli := clients[p.ClientDeviceID]
	if !cli.IsLoggedIn() {
		return nil, ErrRespNotLogin
	}
	jid, err := ParseJID(p.Recipient + "@s.whatsapp.net")
	if errors.Is(err, ErrRecipientNotFound) {
		return nil, ErrRespRecipientNotFound
	}

	msg := &waE2E.Message{Conversation: proto.String(p.Message)}
	if IsOnWhatsapp(cli, jid.ToNonAD().String()) {
		r, e := cli.SendMessage(context.Background(), jid.ToNonAD(), msg)
		fmt.Println(r)
		if e != nil {
			fmt.Println(e)
		}
	}

	resp = &Response{
		Status:  http.StatusOK,
		Message: "message sent",
		Result:  map[string]any{"ok": true},
		Error:   nil,
	}
	return
}
