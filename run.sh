#!/bin/bash
# Scrape the venues in venues.yaml into data/shows.json (no credentials needed).
# For the Tidal artist export, see the README: op run --env-file=.env -- go run ./cmd/export-artists

exec go run . "$@"
