import logging


def report_error(e: Exception):
    logging.error(f"Error occurred: {e}", exc_info=True)
