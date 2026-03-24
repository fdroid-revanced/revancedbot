import logging

logger = logging.getLogger(__name__)


def report_error(
    exception: Exception, message: str = "An unexpected error occurred", **kwargs
):
    """
    Centralized error reporting function.

    If Sentry or any other reporting tool is added, it should be initialized and used here.
    """
    logger.error(f"{message}: {exception}", exc_info=True)
    if kwargs:
        logger.error(f"Context: {kwargs}")
