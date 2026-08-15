# Run the complete Go test suite.
all:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go test ./...

# Query the development fixture roster through the agent.
player:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "查询数据库中所有球队的当前球员名单，并列出球员姓名、球衣号码、打击手、投球手和守备位置。"
