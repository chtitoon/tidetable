#!/bin/bash
# Run TideTable with 1Password secret injection

op run --env-file=.env -- go run main.go
