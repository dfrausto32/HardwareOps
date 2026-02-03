#!/usr/bin/env bash
set -euo pipefail

( cd deploy/compose && docker compose down )
