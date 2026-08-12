"""User-facing domain errors."""


class FanqieError(Exception):
    """Base error for expected client failures."""


class NetworkError(FanqieError):
    """The remote service could not be reached."""


class ParseError(FanqieError):
    """The upstream response no longer matches the known format."""


class LockedChapterError(FanqieError):
    """The requested chapter is locked or requires payment."""


class NotFoundError(FanqieError):
    """The requested book or chapter does not exist."""
