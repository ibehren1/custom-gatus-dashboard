# custom-gatus-dashboard

A compact, wall-display style dashboard for [Gatus](https://github.com/TwiN/gatus).
Every endpoint is a small color-coded pill, grouped by Gatus group, and the page
automatically scales so everything fits on one screen without scrolling.
Clicking a pill opens the standard Gatus endpoint detail page.

It runs as a sidecar container next to Gatus. It reads data from the Gatus REST API
(no database access) and reverse-proxies the rest of the Gatus UI, so the dashboard
and the Gatus detail pages share one origin.

## Pill colors

| Color  | State    | Meaning                                                   |
|--------|----------|-----------------------------------------------------------|
| Green  | Up       | All recent checks succeeded                               |
| Yellow | Flapping | Latest check succeeded, but a recent check failed         |
| Red    | Down     | Latest check failed (also bold with an outline)           |
| Gray   | No data  | No results recorded yet                                   |

"Recent" means the last `HISTORY_SIZE` results. Hover a pill for the full name,
last response time, last check time, recent history, and errors.

Pill labels use the part of the endpoint name before the first ` - `
(for example `Proxmox pve04 - tcp://pve04.iac.lan:22` shows as `Proxmox pve04`).

## Routes

| Path                 | Served by                                    |
|----------------------|----------------------------------------------|
| `/`                  | Dashboard                                    |
| `/dash/static/*`     | Dashboard assets                             |
| `/dash/api/summary`  | Dashboard JSON (grouped, classified pills)   |
| `/dash/healthz`      | Health check                                 |
| everything else      | Reverse-proxied to Gatus (detail pages, API) |

## Configuration

| Variable          | Default             | Description                                         |
|-------------------|---------------------|-----------------------------------------------------|
| `GATUS_URL`       | `http://gatus:8080` | Internal URL of the Gatus container                 |
| `LISTEN_ADDR`     | `:8080`             | Address the dashboard listens on                    |
| `REFRESH_SECONDS` | `15`                | Browser refresh interval                            |
| `HISTORY_SIZE`    | `10`                | Number of recent results used for flap detection    |
| `TITLE`           | Gatus `ui.title`    | Page title; defaults to the title configured in Gatus |

## Deploy

See [`docker-compose.example.yml`](docker-compose.example.yml). Minimal sidecar service:

```yaml
  dashboard:
    image: ghcr.io/ibehren1/custom-gatus-dashboard:latest
    environment:
      - GATUS_URL=http://gatus:8080
    ports:
      - "8083:8080"
    networks:
      - gatus-network
    read_only: true
```

## Development

```sh
go test ./...
docker build -t custom-gatus-dashboard .
```

## Versioning

The version lives in [`VERSION`](VERSION) and is baked into the binary at build time
(shown in the dashboard footer). It starts at `0.0.0`. Every commit increments the
patch number (`z` in `x.y.z`); only the maintainer changes `x.y`.

CI publishes `ghcr.io/ibehren1/custom-gatus-dashboard` as `:latest` and `:<VERSION>`
on pushes to `main`, and `:<x.y.z>` for `vX.Y.Z` tags.

## License

[MIT](LICENSE) © Isaac B. Behrens
