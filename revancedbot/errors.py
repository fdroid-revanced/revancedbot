import logging
from typing import Any

logger = logging.getLogger(__name__)


def report_error(error: Exception, context: dict[str, Any] | None = None) -> None:
    """
    Centralized error reporting function.

    This function should be called from all catch blocks that handle
    unexpected errors instead of silently swallowing them.
    """
    extra = {}
    if context:
        extra["context"] = context

    logger.exception(f"An error occurred: {error}", extra=extra)
