import logging
from typing import Any

logger = logging.getLogger(__name__)

def report_error(err: Exception | Any, context: dict[str, Any] | None = None) -> None:
    """
    Centralized error reporting function.
    All errors should go through this function to allow for centralized handling, logging,
    and sending to external services like Sentry.
    """
    msg = f"Unexpected error occurred: {err}"
    if context:
        msg += f" | Context: {context}"
    logger.error(msg, exc_info=err if isinstance(err, Exception) else None)
