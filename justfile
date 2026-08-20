# Application commands.
mod app 'just/app.just'
# Long-running development dependencies.
mod dep 'just/deps.just'
# Explicit fixed development database maintenance.
mod db 'just/db.just'
# Regenerate PostgreSQL query bindings with the pinned sqlc version.
mod sqlc 'just/sqlc.just'

# Run the named TOML test cases and evaluate their JSONL result.
test test_toml:
    cd {{ justfile_directory() }} && result_path="$(python3 -c 'import sys, uuid; from pathlib import Path; print(Path(".basetion") / "test-results" / f"{Path(sys.argv[1]).stem}-{uuid.uuid4().hex}.jsonl")' "{{ test_toml }}")" && just test-run "{{ test_toml }}" "$result_path" && just test-evaluate "$result_path"

# Run the TOML cases through the Go application and write the requested JSONL.
test-run test_toml result_path:
    cd {{ justfile_directory() }} && just db check && GOTOOLCHAIN=auto go run ./src/cmd/basetion --profile dev test --input "{{ test_toml }}" --output "{{ result_path }}"

# Evaluate an existing batch-test JSONL without running the application again.
test-evaluate result_path:
    cd {{ justfile_directory() }} && python3 ./scripts/evaluate_test_results.py "{{ result_path }}"
