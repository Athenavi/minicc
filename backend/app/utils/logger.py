"""结构化日志配置。"""

from __future__ import annotations

import logging
import sys

from app.utils.config import settings


def setup_logger(name: str = "minicc") -> logging.Logger:
    """配置并返回一个结构化 logger。"""
    logger = logging.getLogger(name)
    logger.setLevel(getattr(logging, settings.log_level.upper(), logging.INFO))

    if not logger.handlers:
        handler = logging.StreamHandler(sys.stdout)
        handler.setFormatter(logging.Formatter(
            fmt="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
            datefmt="%Y-%m-%dT%H:%M:%S%z",
        ))
        logger.addHandler(handler)

    return logger


logger = setup_logger()
