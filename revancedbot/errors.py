import logging

logger = logging.getLogger(__name__)


def report_error(e: Exception) -> None:
    """Centralized error reporting function.

    Currently logs the error, but can be wired up to Sentry or other
    error tracking services in the future.
    """
    logger.exception("An unexpected error occurred: %s", e)
