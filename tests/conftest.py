"""Test isolation: a throwaway database and no live API keys.

Environment variables take precedence over .env in pydantic-settings, so
these must be set before any ``app`` module is imported.
"""

import os
import tempfile

_tmpdir = tempfile.mkdtemp(prefix="stale-detector-tests-")
os.environ["DATABASE_PATH"] = os.path.join(_tmpdir, "test.db")
os.environ["TAVILY_API_KEY"] = ""  # never spend real credits in tests
os.environ["LLM_API_KEY"] = ""
os.environ["API_KEY"] = ""
