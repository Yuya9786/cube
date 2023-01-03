package worker

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Yuya9786/cube/task"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *Api) GetTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(a.Worker.GetTasks())
}

func (a *Api) StartTaskHandler(w http.ResponseWriter, r *http.Request) {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()

	te := task.TaskEvent{}
	if err := d.Decode(&te); err != nil {
		msg := fmt.Sprintf("Error unmarshalling body: %v\n", err)
		log.Printf(msg)
		w.WriteHeader(400)
		e := ErrResponse{
			HTTPStatusCode: 400,
			Message:        msg,
		}
		json.NewEncoder(w).Encode(e)
		return
	}

	a.Worker.AddTask(te)
	log.Printf("Added task %v\n", te.Task.ID)
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(te.Task)
}

func (a *Api) StopTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		log.Printf("No taskID passed in request\n")
		w.WriteHeader(400)
	}

	tID, _ := uuid.Parse(taskID)
	taskToStop, ok := a.Worker.Db[tID]
	if !ok {
		log.Printf("No task with ID %v found", tID)
		w.WriteHeader(404)
	}

	te := task.TaskEvent{
		ID:         uuid.New(),
		Action:     task.Stop,
		Timestatmp: time.Now(),
		Task:       *taskToStop,
	}
	a.Worker.AddTask(te)

	log.Printf("Added task %v to stop container %v\n",
		taskToStop.ID, taskToStop.ContainerId)
	w.WriteHeader(204)
}

func (a *Api) GetStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(a.Worker.Stats)
}
