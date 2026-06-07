"""
数据库字典导出脚本
从开发库导出数据字典，按每50张表拆分为独立 MD 文件

输出: .trae/docs/db-dict/db-dict-01.md, db-dict-02.md, ...
用法: python .trae/skills/db-dict/export_db_dict.py
"""

import sys
from pathlib import Path
from datetime import datetime

import pymysql

DB_CONFIG = {
    "host": "172.16.90.62",
    "port": 3306,
    "user": "root",
    "password": "admin",
    "database": "dev_nndrobot_20251027",
    "charset": "utf8mb4",
}

TABLES_PER_FILE = 50
OUTPUT_DIR = Path(__file__).resolve().parent.parent / ".trae" / "docs" / "db-dict"


def fetch_tables(cur: pymysql.cursors.Cursor, db_name: str) -> list[dict]:
    cur.execute(
        """
        SELECT TABLE_NAME, TABLE_COMMENT, ENGINE, TABLE_ROWS
        FROM information_schema.TABLES
        WHERE TABLE_SCHEMA = %s AND TABLE_TYPE = 'BASE TABLE'
        ORDER BY TABLE_NAME
        """,
        (db_name,),
    )
    return [
        {
            "table_name": r["TABLE_NAME"],
            "table_comment": r["TABLE_COMMENT"],
            "engine": r["ENGINE"],
            "table_rows": r["TABLE_ROWS"],
        }
        for r in cur.fetchall()
    ]


def fetch_columns(cur: pymysql.cursors.Cursor, db_name: str) -> dict[str, list[dict]]:
    cur.execute(
        """
        SELECT
            TABLE_NAME, COLUMN_NAME, ORDINAL_POSITION,
            COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT,
            COLUMN_KEY, EXTRA, COLUMN_COMMENT
        FROM information_schema.COLUMNS
        WHERE TABLE_SCHEMA = %s
        ORDER BY TABLE_NAME, ORDINAL_POSITION
        """,
        (db_name,),
    )
    grouped: dict[str, list[dict]] = {}
    for r in cur.fetchall():
        tbl = r["TABLE_NAME"]
        grouped.setdefault(tbl, []).append(
            {
                "name": r["COLUMN_NAME"],
                "pos": r["ORDINAL_POSITION"],
                "type": r["COLUMN_TYPE"],
                "nullable": r["IS_NULLABLE"],
                "default": r["COLUMN_DEFAULT"],
                "key": r["COLUMN_KEY"],
                "extra": r["EXTRA"],
                "comment": r["COLUMN_COMMENT"],
            }
        )
    return grouped


def build_table_detail(
    t: dict, columns: dict[str, list[dict]]
) -> str:
    tbl_name = t["table_name"]

    lines: list[str] = [
        f"## {tbl_name}",
        "",
        f"**注释**: {t['table_comment'] or '_无_'}  ",
        f"**引擎**: {t['engine']} | **预估行数**: {t['table_rows']}",
        "",
        "| # | 字段名 | 类型 | 可空 | 默认值 | 键 | 注释 |",
        "|---|--------|------|------|--------|-----|------|",
    ]
    for c in columns.get(tbl_name, []):
        key_icon = {"PRI": "PK", "UNI": "UQ", "MUL": "IX"}.get(c["key"], "")
        default_val = str(c["default"]) if c["default"] is not None else "_NULL_"
        comment = c["comment"] or ""
        lines.append(
            f"| {c['pos']} | `{c['name']}` | {c['type']} | {c['nullable']} | {default_val} | {key_icon} | {comment} |"
        )

    lines += ["", "---", ""]
    return "\n".join(lines)


def main() -> None:
    db = DB_CONFIG["database"]
    print(f"连接 {DB_CONFIG['host']}:{DB_CONFIG['port']}/{db} ...")

    try:
        conn = pymysql.connect(
            **DB_CONFIG,
            cursorclass=pymysql.cursors.DictCursor,
            read_timeout=30,
            connect_timeout=10,
        )
    except pymysql.err.OperationalError as e:
        print(f"连接失败: {e}")
        sys.exit(1)

    with conn:
        cur = conn.cursor()
        print("查询表信息...")
        tables = fetch_tables(cur, db)
        print(f"  共 {len(tables)} 张表")
        print("查询字段信息...")
        columns = fetch_columns(cur, db)

    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    total = len(tables)
    file_count = (total + TABLES_PER_FILE - 1) // TABLES_PER_FILE

    for i in range(file_count):
        chunk = tables[i * TABLES_PER_FILE : (i + 1) * TABLES_PER_FILE]
        start = i * TABLES_PER_FILE + 1
        end = start + len(chunk) - 1

        header_lines: list[str] = [
            f"# {db} ({start}-{end}/{total})",
            "",
            f"> {now}",
            "",
            "| # | 表名 | 注释 |",
            "|---|------|------|",
        ]
        for idx, t in enumerate(chunk, start=1):
            header_lines.append(f"| {idx} | {t['table_name']} | {t['table_comment']} |")
        header_lines += ["", "---", ""]

        body = "\n".join(
            build_table_detail(t, columns) for t in chunk
        )
        content = "\n".join(header_lines) + body
        fname = OUTPUT_DIR / f"db-dict-{i + 1:02d}.md"
        fname.write_text(content, encoding="utf-8")
        print(f"[MD] {fname.name} ({fname.stat().st_size / 1024:.1f} KB)")

    print(f"完成! {file_count} 个文件输出到 {OUTPUT_DIR}")


if __name__ == "__main__":
    main()
