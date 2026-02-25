# serveproxy

A lightweight reverse proxy server written in Go.

## Usage

```bash
go run . -config config.yaml
```

## API Endpoints

For services configured with `api: true`:

- `GET /start` - Start the service
- `GET /stop` - Stop the service
- `GET /status` - Get service status

## License

MIT
