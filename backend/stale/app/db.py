"""SQLite database layer for report history and query caching."""

import json
import sqlite3
from typing import Optional, List, Dict, Any
from app.config import settings


def get_db_connection() -> sqlite3.Connection:
    """Create and return a configured SQLite connection."""
    conn = sqlite3.connect(settings.database_path)
    conn.row_factory = sqlite3.Row
    return conn


def init_db() -> None:
    """Initialize database tables with indexes."""
    with get_db_connection() as conn:
        cursor = conn.cursor()
        
        # Reports table
        cursor.execute("""
            CREATE TABLE IF NOT EXISTS reports (
                report_id TEXT PRIMARY KEY,
                checked_at TEXT NOT NULL,
                question TEXT,
                original_answer TEXT NOT NULL,
                overall_freshness_score REAL NOT NULL,
                stale_count INTEGER NOT NULL DEFAULT 0,
                current_count INTEGER NOT NULL DEFAULT 0,
                report_json TEXT NOT NULL
            )
        """)
        cursor.execute("CREATE INDEX IF NOT EXISTS idx_reports_checked_at ON reports(checked_at DESC)")

        # Query cache table with TTL
        cursor.execute("""
            CREATE TABLE IF NOT EXISTS query_cache (
                cache_key TEXT PRIMARY KEY,
                claim_type TEXT NOT NULL,
                created_at REAL NOT NULL,
                expires_at REAL NOT NULL,
                result_json TEXT NOT NULL
            )
        """)
        cursor.execute("CREATE INDEX IF NOT EXISTS idx_cache_expires ON query_cache(expires_at)")
        conn.commit()


# Auto-initialize database on import
init_db()


def save_report(report_dict: Dict[str, Any]) -> None:
    """Save an audited check report to SQLite."""
    with get_db_connection() as conn:
        cursor = conn.cursor()
        summary = report_dict.get("summary", {})
        cursor.execute(
            """
            INSERT OR REPLACE INTO reports (
                report_id, checked_at, question, original_answer,
                overall_freshness_score, stale_count, current_count, report_json
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                report_dict["report_id"],
                report_dict["checked_at"],
                report_dict.get("question"),
                report_dict["original_answer"],
                report_dict["overall_freshness_score"],
                summary.get("stale", 0),
                summary.get("current", 0),
                json.dumps(report_dict),
            ),
        )
        conn.commit()


def get_report_by_id(report_id: str) -> Optional[Dict[str, Any]]:
    """Retrieve a single report by ID."""
    with get_db_connection() as conn:
        cursor = conn.cursor()
        cursor.execute("SELECT report_json FROM reports WHERE report_id = ?", (report_id,))
        row = cursor.fetchone()
        if row:
            return json.loads(row["report_json"])
    return None


def get_reports(limit: int = 20, offset: int = 0) -> List[Dict[str, Any]]:
    """List historical reports with pagination."""
    with get_db_connection() as conn:
        cursor = conn.cursor()
        cursor.execute(
            """
            SELECT report_id, checked_at, question, overall_freshness_score,
                   stale_count, current_count
            FROM reports
            ORDER BY checked_at DESC
            LIMIT ? OFFSET ?
            """,
            (limit, offset),
        )
        rows = cursor.fetchall()
        return [dict(row) for row in rows]
