import logging
from typing import Optional, Dict, Any

logger = logging.getLogger(__name__)


def report_error(e: Exception, context: Optional[Dict[str, Any]] = None):
    """
    Centralized error reporting function.
    All code paths that handle unexpected errors MUST funnel through this function.
    """
    msg = f"Unexpected error occurred: {e}"
    if context:
        msg += f" | Context: {context}"
    logger.error(msg, exc_info=True)
