# Run the complete Go test suite.
all:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go test ./...

# Query the development fixture roster through the agent.
player:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "查询数据库中所有球队的当前球员名单，并列出球员姓名、球衣号码、打击手、投球手和守备位置。"

player-add:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "2026年8月16日，季汉队来了两个新队员，95号关兴是二垒手游击手，打击左右开弓，右手投球；96号张苞是投手和一垒手，左打右投。自动确认。"
