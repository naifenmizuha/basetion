#!/usr/bin/env python3
"""Manage the fixed development PostgreSQL database outside the Go runtime."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path
from typing import Iterable
from urllib.parse import urlsplit, urlunsplit

try:
    import psycopg
    import tomli
except ModuleNotFoundError as error:
    missing = error.name
    raise SystemExit(
        f"缺少 Python 依赖 {missing!r}；请先执行 `python3 -m pip install -r scripts/requirements.txt`"
    ) from error

ROOT = Path(__file__).resolve().parent.parent
CONFIG_FILE = ROOT / "config" / "config.toml"
MIGRATIONS_DIR = ROOT / "sql" / "migrations"
DEVELOPMENT_DATA_DIR = ROOT / "sql" / "development"
TEST_RESET_FILE = ROOT / "sql" / "test" / "reset.sql"


def development_database_url() -> str:
    if not CONFIG_FILE.is_file():
        raise RuntimeError(f"找不到数据库配置文件: {CONFIG_FILE}")
    with CONFIG_FILE.open("rb") as config_file:
        config = tomli.load(config_file)
    try:
        database_url = config["database"]["dev"]["url"]
    except KeyError as error:
        raise RuntimeError(f"配置 {CONFIG_FILE} 中缺少 database.dev.url") from error
    if not isinstance(database_url, str) or not database_url.strip():
        raise RuntimeError(f"配置 {CONFIG_FILE} 中的 database.dev.url 必须是非空字符串")
    return database_url.strip()


def administration_url(database_url: str) -> tuple[str, str]:
    parsed = urlsplit(database_url)
    database_name = parsed.path.lstrip("/")
    if not database_name or "/" in database_name:
        raise RuntimeError("database.dev.url 必须包含单个数据库名")
    return urlunsplit((parsed.scheme, parsed.netloc, "/postgres", parsed.query, "")), database_name


def migration_files() -> Iterable[Path]:
    return sorted(MIGRATIONS_DIR.glob("*.sql"))


def execute_sql_file(connection: psycopg.Connection, path: Path) -> None:
    with connection.cursor() as cursor:
        cursor.execute(path.read_text(encoding="utf-8"))


def create_database(database_url: str) -> None:
    admin_url, database_name = administration_url(database_url)
    with psycopg.connect(admin_url, autocommit=True) as connection:
        with connection.cursor() as cursor:
            cursor.execute("SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = %s)", (database_name,))
            if cursor.fetchone()[0]:
                return
            cursor.execute("SELECT format('CREATE DATABASE %I', %s)", (database_name,))
            cursor.execute(cursor.fetchone()[0])


def migrate_database(database_url: str) -> None:
    with psycopg.connect(database_url) as connection:
        with connection.cursor() as cursor:
            cursor.execute(
                "CREATE TABLE IF NOT EXISTS schema_migrations "
                "(version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)"
            )
        for path in migration_files():
            with connection.cursor() as cursor:
                cursor.execute("SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = %s)", (path.name,))
                if cursor.fetchone()[0]:
                    continue
                cursor.execute(path.read_text(encoding="utf-8"))
                cursor.execute("INSERT INTO schema_migrations(version, applied_at) VALUES (%s, now())", (path.name,))
            connection.commit()


def check_database(database_url: str) -> None:
    with psycopg.connect(database_url) as connection:
        with connection.cursor() as cursor:
            cursor.execute("SELECT 1")
            for path in migration_files():
                cursor.execute("SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = %s)", (path.name,))
                if not cursor.fetchone()[0]:
                    raise RuntimeError(f"数据库缺少迁移: {path.name}；请先执行 `just db dev-db-init`")


def reset_test_data(database_url: str) -> None:
    with psycopg.connect(database_url) as connection:
        execute_sql_file(connection, TEST_RESET_FILE)


def load_development_data(database_url: str) -> None:
    with psycopg.connect(database_url) as connection:
        for path in sorted(DEVELOPMENT_DATA_DIR.glob("*.sql")):
            execute_sql_file(connection, path)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "action",
        choices=("create", "migrate", "init", "reset", "test-reset", "test-load-development", "check"),
    )
    action = parser.parse_args().action
    try:
        database_url = development_database_url()
        if action == "create":
            create_database(database_url)
        elif action == "migrate":
            migrate_database(database_url)
        elif action == "init":
            create_database(database_url)
            migrate_database(database_url)
            reset_test_data(database_url)
            load_development_data(database_url)
        elif action == "reset":
            check_database(database_url)
            reset_test_data(database_url)
            load_development_data(database_url)
        elif action == "test-reset":
            check_database(database_url)
            reset_test_data(database_url)
        elif action == "test-load-development":
            check_database(database_url)
            load_development_data(database_url)
        else:
            check_database(database_url)
    except (OSError, RuntimeError, psycopg.Error) as error:
        print(f"数据库准备失败: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
