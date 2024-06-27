package main

import (
	"database/sql"

	waLog "go.mau.fi/whatsmeow/util/log"
)

type upgradeFunc func(*sql.Tx) error

var Upgrades = [1]upgradeFunc{version1}

type Migration struct {
	db  *sql.DB
	log waLog.Logger
}

func (m *Migration) getVersion() (int, error) {
	_, err := m.db.Exec("CREATE TABLE IF NOT EXISTS whatsmeow_extended_version (version INTEGER)")
	if err != nil {
		return -1, err
	}

	version := 0
	row := m.db.QueryRow("SELECT version FROM whatsmeow_extended_version LIMIT 1")
	if row != nil {
		_ = row.Scan(&version)
	}
	return version, nil
}

func (_ *Migration) setVersion(tx *sql.Tx, version int) error {
	_, err := tx.Exec("DELETE FROM whatsmeow_extended_version")
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO whatsmeow_extended_version (version) VALUES ($1)", version)
	return err
}

func (m *Migration) Upgrade() (err error) {
	var version int
	version, err = m.getVersion()
	if err != nil {
		return err
	}

	for ; version < len(Upgrades); version++ {
		var tx *sql.Tx
		tx, err = m.db.Begin()
		if err != nil {
			return err
		}

		migrateFunc := Upgrades[version]
		m.log.Infof("Upgrading extended database to v%d", version+1)
		err = migrateFunc(tx)

		if err != nil {
			_ = tx.Rollback()
			return err
		}

		if err = m.setVersion(tx, version+1); err != nil {
			return err
		}

		if err = tx.Commit(); err != nil {
			return err
		}
	}

	return
}

func version1(tx *sql.Tx) (err error) {
	_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS "whatsmeow_extended_device" (
		"id" SERIAL NOT NULL,
		"client_device_id" TEXT NOT NULL,

		CONSTRAINT "devices_pkey" PRIMARY KEY ("id")
	);

	ALTER TABLE "whatsmeow_extended_device" ALTER COLUMN "client_device_id" SET DATA TYPE VARCHAR(50);
	
	ALTER TABLE "whatsmeow_extended_device" ADD COLUMN "jid" VARCHAR(50);`)

	return
}
