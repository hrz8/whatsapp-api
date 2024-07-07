package whatsapp

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

const (
	LogLevel       = "DEBUG"
	LogLevelDB     = "INFO"
	LogLevelDevice = "INFO"
)

type EventHandler func(cli *whatsmeow.Client, clientDeviceID string) whatsmeow.EventHandler
type Option func(c *Client)

var (
	ErrClientAlreadyExist = errors.New("whatsapp client with specific id already exist")
	ErrClientNotExist     = errors.New("whatsapp client is not exits")
	ErrQRAlreadyExist     = errors.New("qrcode with specific id already exist")
	ErrQRNotExist         = errors.New("qrcode is not exits")
)

type Client struct {
	mut sync.RWMutex
	WA  map[string]*whatsmeow.Client
	QR  map[string]string

	// customable
	osName     string
	osVersion  [3]uint32
	evtHandler EventHandler

	// default
	container *sqlstore.Container
	mig       *Migration
	repo      *DeviceRepo
	log       waLog.Logger
}

func WithOsInfo(name string, version [3]uint32) Option {
	return func(c *Client) {
		c.osName = fmt.Sprintf("%s@%s", name, fmt.Sprintf("v%d.%d.%d", version[0], version[1], version[2]))
		c.osVersion = version
	}
}

func WithEventHandler(handler EventHandler) Option {
	return func(c *Client) {
		c.evtHandler = handler
	}
}

func NewClient(db *sql.DB, opts ...Option) *Client {
	wa := make(map[string]*whatsmeow.Client)
	qr := make(map[string]string)

	log := waLog.Stdout("Client", LogLevel, true)
	dbLog := waLog.Stdout("Database", LogLevelDB, true)

	waCli := &Client{
		// core
		WA: wa,
		QR: qr,

		// customable
		osName:     "Whatsapp",
		osVersion:  [3]uint32{0, 1, 0},
		evtHandler: nil,

		// default
		container: sqlstore.NewWithDB(db, "postgres", dbLog),
		mig:       &Migration{db, dbLog},
		repo:      &DeviceRepo{db},
		log:       log,
	}

	for _, opt := range opts {
		opt(waCli)
	}

	store.SetOSInfo(waCli.osName, waCli.osVersion)
	return waCli
}

func (c *Client) NewMeow(clientDeviceID string) *whatsmeow.Client {
	meowDevice := c.container.NewDevice()
	cli := c.initMeow(meowDevice, clientDeviceID)
	return cli
}

func (c *Client) initMeow(device *store.Device, clientDeviceID string) *whatsmeow.Client {
	cliLog := waLog.Stdout("Device-"+clientDeviceID, LogLevelDevice, true)
	cli := whatsmeow.NewClient(device, cliLog)
	cli.AddEventHandler(c.defaultEventHandler(clientDeviceID))
	if c.evtHandler != nil {
		cli.AddEventHandler(c.evtHandler(cli, clientDeviceID))
	}

	return cli
}

func (c *Client) Upgrade() (err error) {
	err = c.container.Upgrade()
	if err != nil {
		panic(err)
	}
	err = c.mig.Upgrade()
	if err != nil {
		panic(err)
	}

	return
}

func (c *Client) GetQR(clientDeviceID string) string {
	c.mut.RLock()
	defer c.mut.RUnlock()

	qr := c.QR[clientDeviceID]
	if qr == "" {
		c.log.Warnf("cannot find qrcode fo id: %v", clientDeviceID)
		return ""
	}
	return qr
}

func (c *Client) SetQR(clientDeviceID string, qr string) error {
	curr := c.GetQR(clientDeviceID)
	if curr != "" {
		c.log.Errorf("cannot reassign qrcode for id: %v", clientDeviceID)
		return ErrQRAlreadyExist
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	c.QR[clientDeviceID] = qr
	return nil
}

func (c *Client) ResetQR(clientDeviceID string) error {
	curr := c.GetQR(clientDeviceID)
	if curr == "" {
		c.log.Errorf("cannot find qrcode fo id: %v", clientDeviceID)
		return ErrQRNotExist
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	c.QR[clientDeviceID] = ""
	return nil
}

func (c *Client) Get(clientDeviceID string) *whatsmeow.Client {
	c.mut.RLock()
	defer c.mut.RUnlock()
	cli := c.WA[clientDeviceID]
	if cli == nil {
		c.log.Warnf("cannot find whatsapp client with id: %v", clientDeviceID)
		return nil
	}
	return cli
}

func (c *Client) Set(clientDeviceID string, cli *whatsmeow.Client) error {
	curr := c.Get(clientDeviceID)
	if curr != nil {
		c.log.Errorf("cannot reassigned whatsapp client with id: %v", clientDeviceID)
		return ErrClientAlreadyExist
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	c.WA[clientDeviceID] = cli
	return nil
}

func (c *Client) Reset(clientDeviceID string) error {
	curr := c.Get(clientDeviceID)
	if curr == nil {
		c.log.Errorf("cannot find qrcode for id: %v", clientDeviceID)
		return ErrClientNotExist
	}
	c.mut.Lock()
	defer c.mut.Unlock()
	c.WA[clientDeviceID] = nil
	return nil
}

func (c *Client) Backup() {
	c.log.Infof("attempting back up whatsapp clients...")
	var wg sync.WaitGroup
	for clientDeviceID, cli := range c.WA {
		wg.Add(1)
		go func(clientDeviceID string, cli *whatsmeow.Client) {
			defer wg.Done()
			if cli.Store.ID != nil {
				c.log.Debugf("backing up whatsapp client for id: %v", clientDeviceID)
				c.repo.SetJID(clientDeviceID, cli.Store.ID.String())
			}
		}(clientDeviceID, cli)
	}
	wg.Wait()
	c.log.Infof("backup done!")
}

func (c *Client) defaultEventHandler(clientDeviceID string) whatsmeow.EventHandler {
	return func(evt interface{}) {
		switch evt.(type) {
		case *events.PairSuccess:
			c.ResetQR(clientDeviceID)
		}
	}
}

func (c *Client) Restore() {
	c.log.Infof("attempting to restoring whatsapp clients connections...")
	meowDevices, err := c.container.GetAllDevices()
	if err != nil {
		panic(err)
	}
	var wg sync.WaitGroup
	for _, meowDevice := range meowDevices {
		wg.Add(1)
		go func(meowDevice *store.Device) {
			defer wg.Done()

			jid := meowDevice.ID.String()
			c.log.Debugf("restoring backup for client id: %v", jid)
			extendedDevice, err := c.repo.GetDeviceByJID(jid)
			if err != nil {
				c.log.Warnf("error getting device from db, id: %v", jid)
				return
			}
			cli := c.initMeow(meowDevice, extendedDevice.ClientDeviceID)
			cli.Connect()
			c.Set(extendedDevice.ClientDeviceID, cli)
			c.log.Debugf("reset backup data for client id: %v", jid)
			c.repo.Reset(extendedDevice.ClientDeviceID)
		}(meowDevice)
	}
	wg.Wait()
	c.log.Infof("restore done!")
}
