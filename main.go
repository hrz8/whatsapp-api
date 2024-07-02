package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hrz8/whatsapp-api/internal/session"
	"github.com/hrz8/whatsapp-api/pkg/whatsapp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

var (
	DB_URL = "postgresql://postgres:toor@localhost:5432/whatsapp_api"
)

func eventHandler(cli *whatsmeow.Client, _ string) whatsmeow.EventHandler {
	return func(evt any) {
		switch v := evt.(type) {
		case *events.Message:
			metaParts := []string{fmt.Sprintf("push name: %s", v.Info.PushName), fmt.Sprintf("timestamp: %s", v.Info.Timestamp)}
			if v.Info.Type != "" {
				metaParts = append(metaParts, fmt.Sprintf("type: %s", v.Info.Type))
			}
			if v.Info.Category != "" {
				metaParts = append(metaParts, fmt.Sprintf("category: %s", v.Info.Category))
			}
			if v.IsViewOnce {
				metaParts = append(metaParts, "view once")
			}
			fmt.Printf("Received message %s from %s (%s): %+v\n", v.Info.ID, v.Info.SourceString(), strings.Join(metaParts, ", "), v.Message)

			if !v.Info.IsFromMe &&
				!whatsapp.IsGroupJid(v.Info.Chat.String()) &&
				!strings.Contains(v.Info.SourceString(), "broadcast") &&
				whatsapp.IsOnWhatsapp(cli, v.Info.Sender.ToNonAD().String()) {
				msg := &waE2E.Message{Conversation: proto.String("Sorry, I'm quite busy for now...")}
				_, _ = cli.SendMessage(context.Background(), v.Info.Sender.ToNonAD(), msg)
			}
		}
	}
}

func main() {
	conn, err := pgxpool.New(context.Background(), DB_URL)
	if err != nil {
		panic(err)
	}

	db := stdlib.OpenDBFromPool(conn)

	waCli := whatsapp.NewClient(
		db,
		whatsapp.WithOsInfo(AppOs, AppVersions),
		whatsapp.WithEventHandler(eventHandler),
	)
	waCli.Upgrade()
	defer func() {
		waCli.Backup()
	}()
	waCli.Restore()

	// server
	mux := http.NewServeMux()
	sess := session.NewHandler(waCli)

	mux.Handle("POST /qr", Handler(sess.GenQR))
	mux.Handle("POST /logout", Handler(sess.Logout))
	mux.Handle("POST /send-message", Handler(sess.SendMessage))

	server := http.Server{
		Addr:    fmt.Sprintf(":%s", AppPort),
		Handler: mux,
	}

	// start
	svErr := make(chan error)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			svErr <- err
		}
	}()

	// wait shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case <-c:
		fmt.Println("signal interrupt triggered")
	case err := <-svErr:
		fmt.Println("cannot start server", err.Error())
	}
}
