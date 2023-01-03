package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"

	"github.com/Yuya9786/cube/task"
)

type Worker struct {
	Name      string
	Queue     queue.Queue
	Db        map[uuid.UUID]*task.Task
	TaskCount int
	Stats     *Stats
}

func (w *Worker) CollectState() {
	for {
		log.Println("Collecting stats")
		w.Stats = GetStats()
		w.TaskCount = w.Stats.TaskCount
		time.Sleep(15 * time.Second)
	}
}

func (w *Worker) AddTask(te *task.TaskEvent) {
	w.Queue.Enqueue(te)
}

func (w *Worker) runTask() task.DockerResult {
	te := w.Queue.Dequeue()
	if te == nil {
		log.Println("No tasks in the queue")
		return task.DockerResult{Error: nil}
	}

	taskEventQueued := te.(*task.TaskEvent)

	taskPersisted := w.Db[taskEventQueued.Task.ID]
	if taskPersisted == nil {
		taskPersisted = &taskEventQueued.Task
		w.Db[taskEventQueued.Task.ID] = &taskEventQueued.Task
	}

	var result task.DockerResult
	if taskPersisted.FSM.Can(taskEventQueued.Action) {
		switch taskEventQueued.Action {
		case task.Start:
			result = w.StartTask(&taskEventQueued.Task)
		case task.Stop:
			result = w.StopTask(&taskEventQueued.Task)
		case task.Restart:
			result = w.RestartTask(&taskEventQueued.Task)
		default:
			result.Error = errors.New("We choud not get here")
		}
	} else {
		err := fmt.Errorf("Invalid transition from %v by %v", taskPersisted.FSM.Current(), taskEventQueued.Action)
		result.Error = err
	}

	if result.Error == nil {
		if err := taskPersisted.FSM.Event(context.Background(), taskEventQueued.Action); err != nil {
			result.Error = err
		}
	}

	return result
}

func (w *Worker) RunTasks() {
	for {
		if w.Queue.Len() != 0 {
			result := w.runTask()
			if result.Error != nil {
				log.Printf("Error running task: %v\n", result.Error)
			}
		} else {
			log.Printf("No tasks to process currently\n")
		}
		time.Sleep(10 * time.Second)
	}
}

func (w *Worker) StartTask(t *task.Task) task.DockerResult {
	t.StartTime = time.Now().UTC()
	config := task.NewConfig(t)
	d, err := task.NewDocker(config)
	if err != nil {
		log.Printf("Error preparing for runnig task %v: %v\n", t.ID, err)
		t.FSM.Event(context.Background(), task.Fail)
		w.Db[t.ID] = t
		return task.DockerResult{
			Error: err,
		}
	}

	result := d.Run()
	if result.Error != nil {
		log.Printf("Error running task %v: %v\n", t.ID, result.Error)
		t.FSM.Event(context.Background(), task.Fail)
		w.Db[t.ID] = t
		return result
	}

	t.ContainerId = result.ContainerId
	if err := t.FSM.Event(context.Background(), task.Start); err != nil {
		return task.DockerResult{
			Error: err,
		}
	}
	w.Db[t.ID] = t

	return result
}

func (w *Worker) StopTask(t *task.Task) task.DockerResult {
	config := task.NewConfig(t)
	d, err := task.NewDocker(config)
	if err != nil {
		log.Printf("Error preparing for runnig task %v: %v\n", t.ID, err)
		t.FSM.Event(context.Background(), task.Fail)
		w.Db[t.ID] = t
		return task.DockerResult{
			Error: err,
		}
	}

	result := d.Stop(t.ContainerId)
	if result.Error != nil {
		log.Printf("Error stopping container %v: %v\n", t.ContainerId, result.Error)
		t.FSM.Event(context.Background(), task.Fail)
		w.Db[t.ID] = t
		return result
	}
	t.FinishTime = time.Now().UTC()
	t.FSM.Event(context.Background(), task.Stop)
	w.Db[t.ID] = t
	log.Printf("Stopped and removed container %v for task %v", t.ContainerId, t.ID)

	return result
}

func (w *Worker) RestartTask(t *task.Task) task.DockerResult {
	t.StartTime = time.Now().UTC()
	config := task.NewConfig(t)
	d, err := task.NewDocker(config)
	if err != nil {
		log.Printf("Error preparing for runnig task %v: %v\n", t.ID, err)
		t.FSM.Event(context.Background(), task.Fail)
		w.Db[t.ID] = t
		return task.DockerResult{
			Error: err,
		}
	}

	result := d.Restart(t.ContainerId)
	if result.Error != nil {
		log.Printf("Error running task %v: %v\n", t.ID, result.Error)
		t.FSM.Event(context.Background(), task.Fail)
		w.Db[t.ID] = t
		return result
	}

	t.ContainerId = result.ContainerId
	if err := t.FSM.Event(context.Background(), task.Restart); err != nil {
		return task.DockerResult{
			Error: err,
		}
	}
	w.Db[t.ID] = t

	return result
}

func (w *Worker) GetTasks() []*task.Task {
	tasks := []*task.Task{}
	for _, t := range w.Db {
		tasks = append(tasks, t)
	}
	return tasks
}

func (w *Worker) InspectTask(t *task.Task) task.DockerInspectResponse {
	config := task.NewConfig(t)
	d, err := task.NewDocker(config)
	if err != nil {
		log.Printf("Error preparing for runnig task %v: %v\n", t.ID, err)
		t.FSM.Event(context.Background(), task.Fail)
		w.Db[t.ID] = t
		return task.DockerInspectResponse{
			Error: err,
		}
	}

	return d.Inspect(t.ContainerId)
}

func (w *Worker) UpdateTasks() {
	for {
		log.Println("Checking status of tasks")
		w.updateTasks()
		log.Println("Task updates completed")
		time.Sleep(15 * time.Second)
	}
}

func (w *Worker) updateTasks() {
	for id, t := range w.Db {
		if t.FSM.Current() == task.Running {
			resp := w.InspectTask(t)
			if resp.Error != nil {
				log.Printf("Error inspecting for state of task %s: %v\n", id, resp.Error)
			}

			if resp.Container == nil {
				log.Printf("No container for running task %s\n", id)
				w.Db[id].FSM.Event(context.Background(), task.Fail)
			}

			if resp.Container.State.Status == "exited" {
				log.Printf("Container for task %s in not-running state %s\n", id,
					resp.Container.State.Status)
				w.Db[id].FSM.Event(context.Background(), task.Fail)
			}

			w.Db[id].HostPorts = resp.Container.NetworkSettings.NetworkSettingsBase.Ports
		}
	}
}
