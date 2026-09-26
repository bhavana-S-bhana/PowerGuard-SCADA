from fastapi import FastAPI
from pydantic import BaseModel
from pdf_search import search_alarm

app = FastAPI()


class AlarmRequest(BaseModel):
    alarm_id: str


@app.post("/api/troubleshoot")
def troubleshoot(request: AlarmRequest):

    solution = search_alarm(request.alarm_id)

    return {
        "alarm_id": request.alarm_id,
        "solution": solution
    }