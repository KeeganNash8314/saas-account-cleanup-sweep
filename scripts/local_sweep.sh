#!/bin/sh
set -eu

curl --fail-with-body --request POST http://127.0.0.1:8080/admin/sweep \
  --header 'Content-Type: application/json' \
  --data '{"now":"2026-08-21T12:00:00Z","tenants":[{"id":"new-abandoned","lifecycle":"onboarding","updated_at":"2026-07-01T12:00:00Z"},{"id":"paying","lifecycle":"active","updated_at":"2026-05-01T12:00:00Z"}]}'
