# gowa api

## Usage

request qr code:
```bash
curl -X POST http://localhost:4001/qr --header 'Content-Type: application/json' --data '{"client_device_id": "abc"}'
```

logout:
```bash
curl -X POST http://localhost:4001/logout --header 'Content-Type: application/json' --data '{"client_device_id": "abc"}'
```

send message:
```bash
curl -X POST http://localhost:4001/send-message --header 'Content-Type: application/json' --data '{"recipient": "6283116823235", "message": "your message", "client_device_id": "abc"}'
```
