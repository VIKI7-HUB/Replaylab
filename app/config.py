"""Configuration settings for Stale-Answer Detector."""

from pathlib import Path
from typing import Optional
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Application settings loaded from environment or .env file."""

    model_config = SettingsConfigDict(
        env_file=str(Path(__file__).parent.parent / ".env"),
        env_file_encoding="utf-8",
        extra="ignore",
    )

    # Tavily API Configuration
    tavily_api_key: str = ""
    tavily_base_url: str = "https://api.tavily.com"
    tavily_concurrency: int = 5
    tavily_timeout_seconds: float = 15.0

    # LLM Configuration (Optional, system falls back to heuristic engine)
    llm_base_url: Optional[str] = "https://api.openai.com/v1"
    llm_api_key: Optional[str] = None
    llm_model: str = "gpt-4o-mini"
    llm_timeout_seconds: float = 20.0

    # Database & Storage
    database_path: str = str(Path(__file__).parent.parent / "stale_detector.db")
    cache_enabled: bool = True

    # Security
    api_key: Optional[str] = None  # Optional API key for /v1 routes

    @property
    def has_llm(self) -> bool:
        """Check if an active LLM key is configured."""
        return bool(self.llm_api_key and self.llm_api_key.strip())

    @property
    def has_tavily(self) -> bool:
        """Check if a Tavily API key is configured."""
        return bool(self.tavily_api_key and self.tavily_api_key.strip())


settings = Settings()
