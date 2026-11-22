# AGENTS.md

Guidance for coding agents working in this repository. Read `README.md` for what the project is and the output formats.

## What's here

Two independent scheduled jobs, written in Go, each writing one JSON file:

- **Show listings**: `go run .` (`main.go`) scrapes the venues in `venues.yaml` through `lescraper/` and writes `data/shows.json` via `showexport/`. No credentials. Run by `.github/workflows/daily-shows.yml`.
- **Artist export**: `go run ./cmd/export-artists` reads a Tidal library (`tidal/`), scores it (`artistexport/`) and writes `artists.json`. Needs Tidal secrets. Run by `.github/workflows/daily-artists.yml`.

Neither job reads the other's output.

## Tasks

- **Adding a venue or fixing a broken scraper:** follow [`.claude/skills/add-venue/SKILL.md`](.claude/skills/add-venue/SKILL.md) step by step. Claude Code loads it as the `add-venue` skill; other agents should read the file.

## Commands

```bash
go test ./...                                   # all tests; none hit the network
go test ./lescraper/... ./showexport            # show listings only
go run . -out <scratch>/shows.json              # live scrape; don't overwrite data/shows.json
gofmt -l . && go vet ./...
```

## Rules

- Tests never touch the network. Scrapers fetch through `fetchURL`/`fetchShared` so tests can serve gzipped fixtures from `lescraper/scrapers/testdata/`.
- No headless browser, database or paid service: plain HTTP and JSON only.
- Don't commit `data/shows.json` from a local run. The workflow owns it.
- Never write or commit `data/artists.json` or any other Tidal library data: it's personal and this repo is public. Don't print artist names or IDs in workflow logs either. The `-explain` flag is for local use only.
- Never commit `.env`, tokens or secrets. Local Tidal runs use `op run --env-file=.env -- …` (1Password), which only the user can run interactively.
- When behaviour or an output format changes, update `README.md` in the same change.
