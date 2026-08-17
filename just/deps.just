# Start project dependencies without coupling them to application commands.
up:
    cd {{justfile_directory()}} && podman compose up -d

# Stop project dependencies while retaining their persistent data.
down:
    cd {{justfile_directory()}} && podman compose down
