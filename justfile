default: run

# Start project dependencies without coupling them to application commands.
deps-up:
    docker compose up --detach --wait

# Stop project dependencies while retaining their persistent data.
deps-down:
    docker compose down

run:
    GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile run "介绍一下你能做什么？"

dev:
    GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "介绍一下你能做什么？"

# Ask the agent to query the current player roster from PostgreSQL.
testplayer:
    GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "查询数据库中所有球队的当前球员名单，并列出球员姓名、球衣号码、打击手、投球手和守备位置。"

test:
    GOTOOLCHAIN=auto go test ./...
