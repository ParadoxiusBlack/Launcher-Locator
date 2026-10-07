# Launcher-Locator
Interactive Sniper Elite 5 and Sniper Elite Resistance mission maps documenting and displaying the locations of every PIAT, Panzerfaust, and Flare Gun across each mission. 

## Using the app

Download the executable for your OS from the Releases page and run it. No console or extra programs are needed: it starts a local server and opens the map in your browser. Your custom indicators are stored in a SQLite database in your user config directory (`LauncherLocator/locator.db`) and persist between launches.

- Pick a map from the dropdown; toggle indicator types (PIAT, Panzerfaust, Flare Gun) in the sidebar.
- Click a marker to see its details and optional screenshot.
- **+ Add indicator** then click the map to add a private custom indicator, or submit one for review (with screenshot and details).

## Developer guide

Requires Go. Run `go run . -admin` (or `go test ./...`).

- `-admin` enables developer tools: add maps (upload a map screenshot), add official indicators, approve/reject submissions.
- New indicator types: add an entry to `types` in `assets/seed.json` (or POST to `/api/admin/types`); they appear in filters automatically.
- Bundled maps are listed in `assets/seed.json` (images in `assets/static/maps/`); the bundled maps currently use a placeholder image.
- Flags: `-addr`, `-data`, `-no-browser`.
- Indicator coordinates are stored as fractions of the image size, so they are independent of screenshot resolution.

Layout: `internal/store` (SQLite), `internal/server` (JSON API), `assets/` (embedded web UI + seed data). Tagged releases build single-file binaries via `.github/workflows/release.yml`. Because the UI is a plain web frontend over a JSON API, it can later be hosted online or wrapped for mobile.
