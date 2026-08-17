# Run the application against the persistent database profile.
run:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile run "介绍一下你能做什么？"

# Run the application against a fresh development database.
dev:
    cd {{justfile_directory()}} && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev "介绍一下你能做什么？"
