#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

rm -rf .github cmd internal dist artifacts examples scripts API.html Dockerfile LICENSE Makefile README.html README.md README_EN.md compose.yaml go.mod go.sum openapi.json .env.example .gitignore .dockerignore
printf 'ForumDesk generated files removed. Git metadata was preserved.\n'
