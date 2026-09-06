#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

rm -rf cmd internal data dist artifacts README.html Dockerfile compose.yaml Makefile go.mod .env.example .gitignore
printf 'ForumDesk generated files removed. Git metadata was preserved.\n'
