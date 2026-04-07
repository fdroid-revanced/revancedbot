from dataclasses import dataclass
from typing import Optional

@dataclass
class PatchJob:
    package_id: str
    package_version: Optional[str] # latest if None
