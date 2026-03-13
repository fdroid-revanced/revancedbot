import logging
from typing import Optional, Dict, Any

logger = logging.getLogger(__name__)

def report_error(exception: Exception, context: Optional[Dict[str, Any]] = None) -> None:
    """
    Centralized error reporting function.
    All code paths that handle unexpected errors MUST funnel through this function.
    """
    msg = f"An error occurred: {exception}"
    if context:
        msg += f" | Context: {context}"

    logger.error(msg, exc_info=exception)
