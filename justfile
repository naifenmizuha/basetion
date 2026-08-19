# Application commands.
mod app 'just/app.just'
# Long-running development dependencies.
mod dep 'just/deps.just'
# Explicit fixed development database maintenance.
mod db 'just/db.just'
# Test commands.
mod test 'just/test.just'
# Regenerate PostgreSQL query bindings with the pinned sqlc version.
mod sqlc 'just/sqlc.just'
