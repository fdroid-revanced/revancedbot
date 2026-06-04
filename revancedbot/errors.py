import logging

logger = logging.getLogger(__name__)


def report_error(exception: Exception, context: str = ""):
    """Centralized error reporting function."""
    logger.error(f"Error {context}: {exception}", exc_info=exception)
