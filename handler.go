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

type Response struct {
	Message string `json:"name"`
	Result  any    `json:"result"`
}

type HandlerError struct {
	Code int `json:"code"`
}

type Handler func(w http.ResponseWriter, r *http.Request) (*Response, error)

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	res, err := h(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

type WhatsappHandler struct {
	store *sqlstore.Container
}

// func NewWhatsappHandler(store *sqlstore.Container) *WhatsappHandler {
// 	return &WhatsappHandler{store}
// }

func (h *WhatsappHandler) GenQR(w http.ResponseWriter, r *http.Request) (*Response, error) {
	var p struct {
		ClientDeviceID string `json:"client_device_id"`
	}
	err := json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		// http.Error(w, "server error", http.StatusInternalServerError)
		return nil, errors.New("server error")
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
		// http.Error(w, qr, http.StatusOK)
		return &Response{
			Message: "qr not scanned yet",
			Result:  map[string]string{"qr": qr},
		}, nil
	}

	if cli.IsLoggedIn() {
		// http.Error(w, "already connected", http.StatusOK)
		return nil, errors.New("already connected")
	}

	cli.AddEventHandler(eventHandler(p.ClientDeviceID))
	clients[p.ClientDeviceID] = cli

	ctx, cancel := context.WithCancel(context.Background())
	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		defer cancel()
		// http.Error(w, "already connected", http.StatusOK)
		return nil, errors.New("already connected")
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
	// http.Error(w, qr, http.StatusOK)
	return &Response{
		Message: "success create qr",
		Result:  map[string]string{"qr": qr},
	}, nil
}

func (_ *WhatsappHandler) Logout(w http.ResponseWriter, r *http.Request) (*Response, error) {
	var p struct {
		DeviceID string `json:"client_device_id"`
	}
	err := json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		// http.Error(w, "server error", http.StatusInternalServerError)
		return nil, errors.New("server error")
	}

	cli := clients[p.DeviceID]
	cli.Logout()
	// http.Error(w, "ok", http.StatusOK)
	return &Response{
		Message: "logout success",
		Result:  map[string]any{"ok": true},
	}, nil
}
