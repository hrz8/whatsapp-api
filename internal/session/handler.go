package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/hrz8/whatsapp-api/pkg/response"
	"github.com/hrz8/whatsapp-api/pkg/whatsapp"
	"github.com/mdp/qrterminal/v3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

type Handler struct {
	waCli *whatsapp.Client
}

func NewHandler(waCli *whatsapp.Client) *Handler {
	return &Handler{waCli}
}

type ClientPayload struct {
	ClientDeviceID string `json:"client_device_id"`
}

func (h *Handler) GenQR(w http.ResponseWriter, r *http.Request) (resp *response.Response, err error) {
	var p ClientPayload
	err = json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		return nil, response.ErrRespServerUnexpected
	}

	var cli *whatsmeow.Client

	cli = h.waCli.Get(p.ClientDeviceID)
	qr := h.waCli.GetQR(p.ClientDeviceID)

	if cli == nil {
		cli = h.waCli.NewMeow(p.ClientDeviceID)
		h.waCli.Set(p.ClientDeviceID, cli)
	}

	if qr != "" {
		resp = &response.Response{
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
					h.waCli.Reset(p.ClientDeviceID)
					h.waCli.ResetQR(p.ClientDeviceID)
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
	h.waCli.SetQR(p.ClientDeviceID, qr)
	resp = &response.Response{
		Status:  http.StatusOK,
		Message: "success create qr",
		Result:  map[string]string{"qr": qr},
		Error:   nil,
	}
	return
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) (resp *response.Response, err error) {
	var p ClientPayload
	err = json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		return nil, response.ErrRespServerUnexpected
	}

	cli := h.waCli.Get(p.ClientDeviceID)
	if cli == nil {
		return nil, ErrRespNotLogin
	}
	if !cli.IsLoggedIn() {
		return nil, ErrRespNotLogin
	}
	cli.Logout()

	resp = &response.Response{
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

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) (resp *response.Response, err error) {
	var p SendMessagePayload
	err = json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		return nil, response.ErrRespServerUnexpected
	}

	cli := h.waCli.Get(p.ClientDeviceID)
	if cli == nil {
		return nil, ErrRespNotLogin
	}
	if !cli.IsLoggedIn() {
		return nil, ErrRespNotLogin
	}

	jid, err := whatsapp.ParseJID(p.Recipient + "@s.whatsapp.net")
	if errors.Is(err, ErrRecipientNotFound) {
		return nil, ErrRespRecipientNotFound
	}

	msg := &waE2E.Message{Conversation: proto.String(p.Message)}
	if whatsapp.IsOnWhatsapp(cli, jid.ToNonAD().String()) {
		r, e := cli.SendMessage(context.Background(), jid.ToNonAD(), msg)
		fmt.Println(r)
		if e != nil {
			fmt.Println(e)
		}
	}

	resp = &response.Response{
		Status:  http.StatusOK,
		Message: "message sent",
		Result:  map[string]any{"ok": true},
		Error:   nil,
	}
	return
}
