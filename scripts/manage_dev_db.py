#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "psycopg[binary]==3.2.10",
# ]
# ///
"""Rebuild the fixed development PostgreSQL database from repository SQL."""

from __future__ import annotations

import argparse
import sys
import tomllib
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

try:
    import psycopg
    from psycopg import sql
except ModuleNotFoundError as error:
    missing = error.name
    raise SystemExit(
        f"缺少 Python 依赖 {missing!r}；请通过 `uv run --script scripts/manage_dev_db.py` 运行"
    ) from error

ROOT = Path(__file__).resolve().parent.parent
CONFIG_FILE = ROOT / "config" / "config.toml"
MIGRATIONS_DIR = ROOT / "sql" / "migrations"
DEVELOPMENT_DATA_DIR = ROOT / "sql" / "development"


def development_database_url() -> str:
    if not CONFIG_FILE.is_file():
        raise RuntimeError(f"找不到数据库配置文件: {CONFIG_FILE}")
    with CONFIG_FILE.open("rb") as config_file:
        config = tomllib.load(config_file)
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


def execute_sql_file(connection: psycopg.Connection, path: Path) -> None:
    with connection.cursor() as cursor:
        cursor.execute(path.read_text(encoding="utf-8"))


def recreate_database(database_url: str) -> None:
    admin_url, database_name = administration_url(database_url)
    with psycopg.connect(admin_url, autocommit=True) as connection:
        with connection.cursor() as cursor:
            cursor.execute(
                "SELECT pg_terminate_backend(pid) FROM pg_stat_activity "
                "WHERE datname = %s AND pid <> pg_backend_pid()",
                (database_name,),
            )
            cursor.execute(sql.SQL("DROP DATABASE IF EXISTS {}").format(sql.Identifier(database_name)))
            cursor.execute(sql.SQL("CREATE DATABASE {}").format(sql.Identifier(database_name)))


def load_schema(database_url: str) -> None:
    with psycopg.connect(database_url) as connection:
        for path in sorted(MIGRATIONS_DIR.glob("*.sql")):
            execute_sql_file(connection, path)


def load_development_data(database_url: str) -> None:
    with psycopg.connect(database_url) as connection:
        for path in sorted(DEVELOPMENT_DATA_DIR.glob("*.sql")):
            execute_sql_file(connection, path)


def reset(database_url: str) -> None:
    recreate_database(database_url)
    load_schema(database_url)
    load_development_data(database_url)
    check(database_url)


def check(database_url: str) -> None:
    """Verify that the fixed development fixture has the minimum game data."""
    with psycopg.connect(database_url) as connection:
        with connection.cursor() as cursor:
            cursor.execute("SELECT COUNT(*) FROM teams WHERE deleted_at IS NULL AND active")
            teams = cursor.fetchone()[0]
            cursor.execute("SELECT COUNT(*) FROM matches WHERE deleted_at IS NULL AND status = 3")
            finals = cursor.fetchone()[0]
            cursor.execute("SELECT COUNT(DISTINCT match_id) FROM lineups WHERE deleted_at IS NULL")
            lineup_matches = cursor.fetchone()[0]
            cursor.execute("SELECT COUNT(*) FROM plays WHERE deleted_at IS NULL")
            plays = cursor.fetchone()[0]
    if teams < 2 or finals < 1 or lineup_matches < 1 or plays < 1:
        raise RuntimeError("development fixture 不完整：需要两支启用球队、一场已结束比赛、阵容和有效 Play")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("reset", "check"))
    action = parser.parse_args().action
    try:
        if action == "reset":
            reset(development_database_url())
        elif action == "check":
            check(development_database_url())
    except (OSError, RuntimeError, psycopg.Error) as error:
        print(f"数据库准备失败: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
