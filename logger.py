import logging

class Logger:
    def __init__(self, level=logging.INFO):
        logging.basicConfig(level=level)
        self.logger = logging.getLogger(__name__)

    def debug(self, message):
        self.logger.debug(message)

    def info(self, message):
        self.logger.info(message)

    def error(self, message):
        self.logger.error(message)

    def warning(self, message):
        self.logger.warning(message)

    def critical(self, message):
        self.logger.critical(message)

logger = Logger()

if __name__ == "__main__":
    logger.info("Info message...")
    logger.debug("Debug message...")
    logger.error("Error message...")
    logger.warning("Warning message...")
    logger.critical("Critical message...")
