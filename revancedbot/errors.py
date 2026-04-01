import logging

logger = logging.getLogger(__name__)


def report_error(e: Exception):
    logger.error("An unexpected error occurred: %s", e, exc_info=True)
