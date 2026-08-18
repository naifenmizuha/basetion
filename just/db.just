# Create the fixed development database from database.dev in config/config.toml.
create:
    cd {{justfile_directory()}} && python3 ./scripts/manage_dev_db.py create

# Create, migrate, and load the fixed development database with demo data.
init:
    cd {{justfile_directory()}} && python3 ./scripts/manage_dev_db.py init

# Clear fixed development business data and reload demo data.
reset:
    cd {{justfile_directory()}} && python3 ./scripts/manage_dev_db.py reset

# Verify the fixed integration database is ready without creating or deleting it.
check:
    cd {{justfile_directory()}} && python3 ./scripts/manage_dev_db.py check

# Clear test data without loading development fixtures.
test-reset:
    cd {{justfile_directory()}} && python3 ./scripts/manage_dev_db.py test-reset
