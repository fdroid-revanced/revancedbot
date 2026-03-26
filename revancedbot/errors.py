import logging

logger = logging.getLogger(__name__)

def report_error(e: Exception):
    """
    Centralized error reporting function.
    All errors caught should funnel through this function.
    """
    logger.error("An unexpected error occurred", exc_info=e)
