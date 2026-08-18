# Run the complete Go test suite against the prepared fixed development database.
all:
    cd {{justfile_directory()}} && just db check && GOTOOLCHAIN=auto go test ./...

# Query the development fixture roster through the agent.
player:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "查询数据库中所有球队的当前球员名单，并列出球员姓名、球衣号码、打击手、投球手和守备位置。"

player-add:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "2026年8月16日，季汉队来了两个新队员，95号关兴是二垒手游击手，打击左右开弓，右手投球；96号张苞是投手和一垒手，左打右投。自动确认。"

game:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "最近蜀汉队和曹魏队的比赛结果咋样，有啥亮眼表现吗？"

training:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "分析一下最近3天曹魏队队员的训练内容，看看有谁在偷懒"

training-mod:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "我是刘备，我今天(8月18日)投球了150个，感觉投完手有点痛，记一下，自动确认"

temp:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "查一下蜀汉队最近一场已结束的比赛：告诉我对手和最终比分，列出双方首发阵容，并按顺序概括这场比赛的所有打席"

complex:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "查看迄今为止的所有比赛数据，并分析出球员在进攻和防守上的数据。分成进攻表现、投球表现（仅投手）、守备表现（仅野手）三类，列出每个球员的各项数据，并按表现从高到低排序。"
