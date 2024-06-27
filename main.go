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
	ErrServerUnexpected = errors.New("server error occurred")
	ErrAlreadyConnected = errors.New("device already connected")
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
	mux.Handle("POST /qr", Handler(GenQR(container)))
	mux.Handle("POST /logout", Handler(Logout()))

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
