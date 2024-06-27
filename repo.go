package main

import "database/sql"

type Device struct {
	ID             int    `db:"id"`
	JID            string `db:"jid"`
	ClientDeviceID string `db:"client_device_id"`
}

type DeviceRepo struct {
	db *sql.DB
}

func (r *DeviceRepo) GetDeviceByJID(jid string) (*Device, error) {
	row := r.db.QueryRow(`SELECT id, client_device_id, jid from whatsmeow_extended_device`)
	var i Device
	err := row.Scan(
		&i.ID,
		&i.ClientDeviceID,
		&i.JID,
	)
	return &i, err
}

func (r *DeviceRepo) SetJID(clientDeviceID string, jid string) (int, error) {
	row := r.db.QueryRow(`INSERT INTO
		whatsmeow_extended_device (
			client_device_id,
			jid
		)
		VALUES ($1, $2) RETURNING id`,
		clientDeviceID,
		jid,
	)
	var id int
	err := row.Scan(&id)
	return id, err
}

func (r *DeviceRepo) Reset(clientDeviceID string) error {
	_, err := r.db.Query("DELETE FROM whatsmeow_extended_device WHERE client_device_id = $1", clientDeviceID)
	return err
}
