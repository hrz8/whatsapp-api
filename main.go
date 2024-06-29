package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var (
	DB_URL = "postgresql://postgres:toor@localhost:5432/whatsapp_api"
)

var clients = make(map[string]*whatsmeow.Client)
var qrs = make(map[string]string)

func isGroupJid(jid string) bool {
	return strings.Contains(jid, "@g.us")
}

func IsOnWhatsapp(waCli *whatsmeow.Client, jid string) bool {
	if strings.Contains(jid, "@s.whatsapp.net") {
		data, err := waCli.IsOnWhatsApp([]string{jid})
		if err != nil {
			fmt.Println("recipient not exist")
			return false
		}

		for _, v := range data {
			if !v.IsIn {
				return false
			}
		}
	}

	return true
}

func ParseJID(arg string) (types.JID, error) {
	if arg[0] == '+' {
		arg = arg[1:]
	}
	if !strings.ContainsRune(arg, '@') {
		return types.NewJID(arg, types.DefaultUserServer), nil
	} else {
		recipient, err := types.ParseJID(arg)
		if err != nil {
			fmt.Printf("invalid JID %s: %v", arg, err)
			return recipient, ErrRecipientNotFound
		} else if recipient.User == "" {
			fmt.Printf("invalid JID %v: no server specified", arg)
			return recipient, ErrRecipientNotFound
		}
		return recipient, nil
	}
}

func eventHandler(clientDeviceID string) func(evt interface{}) {
	return func(evt interface{}) {
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

			cli := clients[clientDeviceID]
			if cli == nil {
				return
			}
			if !isGroupJid(v.Info.Chat.String()) &&
				!strings.Contains(v.Info.SourceString(), "broadcast") &&
				IsOnWhatsapp(cli, v.Info.Sender.ToNonAD().String()) {
				msg := &waE2E.Message{Conversation: proto.String("Maaf sedang sibuk")}
				_, _ = cli.SendMessage(context.Background(), v.Info.Sender.ToNonAD(), msg)
			}
		case *events.PairSuccess:
			qrs[clientDeviceID] = ""
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
	store.SetOSInfo(fmt.Sprintf("%s@%s", AppOs, AppVersion), AppVersions)
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
	defer cleanup(repo)

	// restore all connections
	meowDevices, err := container.GetAllDevices()
	if err != nil {
		panic(err)
	}
	var wg sync.WaitGroup
	for _, device := range meowDevices {
		wg.Add(1)
		go func(device *store.Device) {
			defer wg.Done()

			jid := device.ID.String()
			fmt.Println("restoring backup for", jid)
			dvc, err := repo.GetDeviceByJID(jid)
			if err != nil {
				fmt.Println("error getting device:", err)
				return
			}
			cli := whatsmeow.NewClient(device, waLog.Stdout(dvc.ClientDeviceID, "INFO", true))
			cli.AddEventHandler(eventHandler(dvc.ClientDeviceID))
			cli.Connect()
			clients[dvc.ClientDeviceID] = cli
			fmt.Println("delete backup", jid, dvc.ClientDeviceID)
			repo.Reset(dvc.ClientDeviceID)
		}(device)
	}
	wg.Wait()

	// server
	mux := http.NewServeMux()
	wa := &WhatsappHandler{container}

	mux.Handle("POST /qr", Handler(wa.GenQR))
	mux.Handle("POST /logout", Handler(wa.Logout))
	mux.Handle("POST /send-message", Handler(wa.SendMessage))

	server := http.Server{
		Addr:    fmt.Sprintf(":%s", AppPort),
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

func cleanup(repo *DeviceRepo) {
	for deviceClientID, cli := range clients {
		if cli.Store.ID != nil {
			fmt.Println("backing up for", deviceClientID)
			repo.SetJID(deviceClientID, cli.Store.ID.String())
		}
	}
}
