# whatsmeow example

## Usage

request qr code:
```
curl -X POST http://localhost:4001/qr --header 'Content-Type: application/json' --data '{"client_device_id": "abc"}'
```

logout:
```
curl -X POST http://localhost:4001/logout --header 'Content-Type: application/json' --data '{"client_device_id": "abc"}'
```
