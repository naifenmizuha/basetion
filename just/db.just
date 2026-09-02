# Recreate the fixed development database from current migrations and demo data.
reset:
    @echo "正在重建 basetion_dev 并载入演示数据；这会删除该开发库中的全部既有数据。"
    cd {{ justfile_directory() }} && uv run --script ./scripts/manage_dev_db.py reset

check:
    cd {{ justfile_directory() }} && uv run --script ./scripts/manage_dev_db.py check
