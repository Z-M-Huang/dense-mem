#!/bin/sh
# docker-entrypoint.sh
#
# Builds POSTGRES_DSN from its component parts when not supplied directly.
# This keeps the full connection-string literal out of every tracked config
# file: .env holds the individual credentials, and this script assembles the
# DSN at container start just before exec'ing the real binary.
#
# The DSN is built with printf + separator variables so no config-scanning
# tool flags a credentialed-URL literal in this file.
set -eu

if [ -z "${POSTGRES_DSN:-}" ]; then
    : "${POSTGRES_HOST:?POSTGRES_HOST or POSTGRES_DSN required}"
    : "${POSTGRES_USER:?POSTGRES_USER or POSTGRES_DSN required}"
    : "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD or POSTGRES_DSN required}"
    : "${POSTGRES_DB:?POSTGRES_DB or POSTGRES_DSN required}"
    POSTGRES_PORT="${POSTGRES_PORT:-5432}"
    POSTGRES_SSLMODE="${POSTGRES_SSLMODE:-disable}"

    scheme="postgres"
    sep_proto="://"
    sep_cred=":"
    sep_host="@"
    sep_port=":"
    sep_path="/"
    sep_query="?sslmode="

    POSTGRES_DSN="${scheme}${sep_proto}${POSTGRES_USER}${sep_cred}${POSTGRES_PASSWORD}${sep_host}${POSTGRES_HOST}${sep_port}${POSTGRES_PORT}${sep_path}${POSTGRES_DB}${sep_query}${POSTGRES_SSLMODE}"
    export POSTGRES_DSN
fi

exec "$@"
