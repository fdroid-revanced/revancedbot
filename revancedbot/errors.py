import logging
import traceback

logger = logging.getLogger(__name__)

def report_error(e: Exception, context: str = "") -> None:
    """
    Centralized error-reporting function.

    All unexpected exceptions and empty catch blocks must be routed through this function.
    It ensures errors are captured with their full traceback and any additional context.
    If Sentry or another tracking backend is added later, it should be hooked in here.

    Args:
        e (Exception): The exception that was caught.
        context (str, optional): Additional context about where or why the error occurred.
    """
    error_msg = f"Unexpected error: {str(e)}"
    if context:
        error_msg = f"{context} - {error_msg}"

    logger.error(error_msg)
    logger.error(traceback.format_exc())
