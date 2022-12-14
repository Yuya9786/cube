package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Yuya9786/cube/task"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

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

	a.Manager.AddTask(&te)
	log.Printf("Added task %v\n", te.Task.ID)
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(te.Task)
}

func (a *Api) GetTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(a.Manager.GetTasks())
}

func (a *Api) StopTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		log.Println("No taskID passed in request")
		w.WriteHeader(400)
	}

	tID, _ := uuid.Parse(taskID)
	taskToStop, ok := a.Manager.TaskDb[tID]
	if !ok {
		log.Printf("No task with ID %v found\n", tID)
		w.WriteHeader(404)
	}

	te := task.TaskEvent{
		ID:         uuid.New(),
		Action:     task.Stop,
		Timestatmp: time.Now(),
		Task:       *taskToStop,
	}

	if err := taskToStop.FSM.Event(context.Background(), "Stop"); err != nil {
		log.Printf("Unable to transit state from %s by \"Stop\"\n", taskToStop.FSM.Current())
	}

	a.Manager.AddTask(&te)

	log.Printf("Added task event %v to stop task %v\n", te.ID, taskToStop.ID)
	w.WriteHeader(204)
}
