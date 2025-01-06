from datetime import datetime
from pydantic import BaseModel


class Step(BaseModel):
    id: int
    name: str
    description: str
    order: int
    project_id: int
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True
