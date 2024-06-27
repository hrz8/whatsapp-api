package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

func eventHandler(clientDeviceID string) func(evt interface{}) {
	return func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			fmt.Println("Received a message!", v.Message)
		case *events.PairSuccess:
			qrs[clientDeviceID] = ""
		}
	}
}

func main() {
	dbLog := waLog.Stdout("Database", "INFO", true)
	cliLog := waLog.Stdout("abc", "INFO", true)

	conn, err := pgxpool.New(context.Background(), DB_URL)
	if err != nil {
		panic(err)
	}

	db := stdlib.OpenDBFromPool(conn)
	store.SetOSInfo("YourApp", [3]uint32{0, 1, 0})
	container := sqlstore.NewWithDB(db, "postgres", dbLog)
	err = container.Upgrade()
	if err != nil {
		panic(err)
	}
	m := &Migration{db, dbLog}
	err = m.Upgrade()
	if err != nil {
		fmt.Println("migration upgrade failed", err)
	}
	repo := &DeviceRepo{db}

	// restore all connections
	devices, err := container.GetAllDevices()
	if err != nil {
		panic(err)
	}
	for _, device := range devices {
		var dvc *Device
		jid := device.ID.String()
		fmt.Println("restoring backup for", jid)
		dvc, err = repo.GetDeviceByJID(jid)
		if err != nil {
			continue
		}
		cli := whatsmeow.NewClient(device, cliLog)
		cli.AddEventHandler(eventHandler(dvc.ClientDeviceID))
		cli.Connect()
		clients[dvc.ClientDeviceID] = cli
		fmt.Println("delete backup", jid, dvc.ClientDeviceID)
		repo.Reset(dvc.ClientDeviceID)
	}

	// server
	mux := http.NewServeMux()
	wa := &WhatsappHandler{container}

	mux.Handle("POST /qr", Handler(wa.GenQR))
	mux.Handle("POST /logout", Handler(wa.Logout))

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

	// backup
	defer func() {
		for deviceClientID, cli := range clients {
			if cli.Store.ID != nil {
				fmt.Println("backing up for", deviceClientID)
				repo.SetJID(deviceClientID, cli.Store.ID.String())
			}
		}
	}()
}
