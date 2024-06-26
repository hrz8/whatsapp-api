package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"
	"github.com/skip2/go-qrcode"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var (
	DB_URL = "postgresql://postgres:toor@localhost:5432/whatsapp_api"
)

var clients = make(map[string]*whatsmeow.Client)
var qrs = make(map[string]string)

func eventHandler(deviceID string) func(evt interface{}) {
	return func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			fmt.Println("Received a message!", v.Message.GetExtendedTextMessage().Text)
		case *events.PairSuccess:
			qrs[deviceID] = ""
		}
	}
}

func main() {
	dbLog := waLog.Stdout("Database", "INFO", true)
	conn, err := pgxpool.New(context.Background(), DB_URL)
	if err != nil {
		panic(err)
	}
	db := stdlib.OpenDBFromPool(conn)
	store.SetOSInfo("AiConec", [3]uint32{0, 1, 0})
	container := sqlstore.NewWithDB(db, "postgres", dbLog)
	err = container.Upgrade()
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /qr", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			DeviceID string `json:"device_id"`
		}
		err := json.NewDecoder(r.Body).Decode(&p)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		var cli *whatsmeow.Client
		var deviceStore *store.Device

		cli = clients[p.DeviceID]
		qr := qrs[p.DeviceID]

		if cli == nil {
			deviceStore = container.NewDevice()
			cli = whatsmeow.NewClient(deviceStore, waLog.Stdout(p.DeviceID, "INFO", true))
		}

		if qr != "" {
			http.Error(w, qr, http.StatusOK)
			return
		}

		if cli.IsLoggedIn() {
			http.Error(w, "already connected", http.StatusOK)
			return
		}

		cli.AddEventHandler(eventHandler(p.DeviceID))
		clients[p.DeviceID] = cli

		ctx, cancel := context.WithCancel(context.Background())
		qrChan, err := cli.GetQRChannel(ctx)
		if err != nil {
			defer cancel()
			http.Error(w, "already connected", http.StatusOK)
			return
		}

		chImg := make(chan string)

		go func() {
			evt := <-qrChan
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				go func() {
					time.Sleep(evt.Timeout - 10*time.Second)
					if !cli.IsLoggedIn() {
						clients[p.DeviceID] = nil
						qrs[p.DeviceID] = ""
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
		qrs[p.DeviceID] = qr
		http.Error(w, qr, http.StatusOK)
	})

	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			DeviceID string `json:"device_id"`
		}
		err := json.NewDecoder(r.Body).Decode(&p)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		cli := clients[p.DeviceID]
		cli.Disconnect()
		http.Error(w, "ok", http.StatusOK)
	})

	server := http.Server{
		Addr:    ":4001",
		Handler: mux,
	}

	svErr := make(chan error)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			svErr <- err
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case <-c:
		fmt.Println("signal interrupt triggered")
	case err := <-svErr:
		fmt.Println("cannot start server", err.Error())
	}
}

func example(deviceStore *store.Device) {
	var err error
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	// client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				// e.g. qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}
}
