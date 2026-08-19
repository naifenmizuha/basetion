sqlc_version := "v1.30.0"

# Generate pgx/v5 bindings from migrations and named SQL queries. This is
# deliberately explicit: application and test commands never run generation.
generate:
    cd {{justfile_directory()}} && go run github.com/sqlc-dev/sqlc/cmd/sqlc@{{sqlc_version}} generate

# Fail when checked-in bindings are stale after regeneration.
check:
    cd {{justfile_directory()}} && just sqlc generate && git diff --exit-code -- src/internal/infra/postgres/sqlcgen
