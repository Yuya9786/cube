package manager

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Yuya9786/cube/task"
	"github.com/Yuya9786/cube/worker"
	"github.com/docker/go-connections/nat"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

type Manager struct {
	Pending       queue.Queue
	TaskDb        map[uuid.UUID]*task.Task
	EventDb       map[uuid.UUID]*task.TaskEvent
	Workers       []string
	WorkerTaskMap map[string][]uuid.UUID
	TaskWorkerMap map[uuid.UUID]string
	LastWorker    int
}

func New(workers []string) *Manager {
	taskDb := make(map[uuid.UUID]*task.Task)
	eventDb := make(map[uuid.UUID]*task.TaskEvent)
	workerTaskMap := make(map[string][]uuid.UUID)
	taskWorkerMap := make(map[uuid.UUID]string)
	for _, worker := range workers {
		workerTaskMap[worker] = []uuid.UUID{}
	}

	return &Manager{
		Pending:       *queue.New(),
		TaskDb:        taskDb,
		EventDb:       eventDb,
		Workers:       workers,
		WorkerTaskMap: workerTaskMap,
		TaskWorkerMap: taskWorkerMap,
	}
}

func (m *Manager) SelectWorker() string {
	var newWorker int
	if m.LastWorker+1 < len(m.Workers) {
		newWorker = m.LastWorker + 1
		m.LastWorker++
	} else {
		newWorker = 0
		m.LastWorker = 0
	}

	return m.Workers[newWorker]
}

func (m *Manager) ProcessTasks() {
	for {
		log.Println("Processing any task in the queue")
		m.SendTask()
		log.Println("Sleeping for 10 seconds")
		time.Sleep(10 * time.Second)
	}
}

func (m *Manager) SendTask() {
	if m.Pending.Len() > 0 {
		w := m.SelectWorker()

		e := m.Pending.Dequeue()
		te := e.(task.TaskEvent)
		t := te.Task
		log.Printf("Pulled %v off pending queue\n", t)

		m.EventDb[te.ID] = &te
		m.WorkerTaskMap[w] = append(m.WorkerTaskMap[w], t.ID)
		m.TaskWorkerMap[t.ID] = w

		t.State = task.Scheduled
		m.TaskDb[t.ID] = &t

		data, err := json.Marshal(te)
		if err != nil {
			log.Printf("Unable to marshal task object: %v\n", err)
		}

		url := fmt.Sprintf("http://%s/tasks", w)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Printf("Error connecting to %v: %v\n", w, err)
			m.Pending.Enqueue(te)
			return
		}

		d := json.NewDecoder(resp.Body)
		if resp.StatusCode != http.StatusCreated {
			e := worker.ErrResponse{}
			err := d.Decode(&e)
			if err != nil {
				log.Printf("Error decoding response: %v\n", err)
				return
			}
			log.Printf("Response error (%d): %s\n", e.HTTPStatusCode, e.Message)
			return
		}

		t = task.Task{}
		err = d.Decode(&t)
		if err != nil {
			log.Printf("Error decoding response: %v\n", err)
			return
		}
		log.Printf("%#v\n", t)
	} else {
		log.Println("No work in the queue")
	}
}

func (m *Manager) updateTasks() {
	for _, w := range m.Workers {
		log.Printf("Checking worker %v for task updates", w)
		url := fmt.Sprintf("http://%s/tasks", w)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error connecting to %v: %v\n", w, err)
			continue
		}

		d := json.NewDecoder(resp.Body)
		if resp.StatusCode != http.StatusOK {
			e := worker.ErrResponse{}
			err := d.Decode(&e)
			if err != nil {
				log.Printf("Error decoding response: %v\n", err)
				return
			}
			log.Printf("Response error (%d): %s\n", e.HTTPStatusCode, e.Message)
			return
		}

		var tasks []*task.Task
		err = d.Decode(&tasks)
		if err != nil {
			log.Printf("Error decoding response: %v\n", err)
			return
		}
		for _, t := range tasks {
			log.Printf("Attempt to update task %v", t.ID)

			_, ok := m.TaskDb[t.ID]
			if !ok {
				log.Printf("Task with %v not found\n", t.ID)
				return
			}
			if m.TaskDb[t.ID].State != t.State {
				m.TaskDb[t.ID].State = t.State
			}

			m.TaskDb[t.ID].StartTime = t.StartTime
			m.TaskDb[t.ID].FinishTime = t.FinishTime
			m.TaskDb[t.ID].ContainerId = t.ContainerId
		}
	}
}

func (m *Manager) UpdateTasks() {
	for {
		log.Println("Checking for task updates from workers")
		m.updateTasks()
		log.Println("Task updates completed")
		log.Println("Sleeping for 10 seconds")
		time.Sleep(time.Second * 10)
	}
}
func (m *Manager) AddTask(te task.TaskEvent) {
	m.Pending.Enqueue(te)
}

func (m *Manager) GetTasks() []*task.Task {
	tasks := []*task.Task{}
	for _, t := range m.TaskDb {
		tasks = append(tasks, t)
	}

	return tasks
}

func getHostPort(ports nat.PortMap) *string {
	for k := range ports {
		return &ports[k][0].HostPort
	}
	return nil
}

func (m *Manager) checkTaskHealth(t task.Task) error {
	log.Printf("Calling health check for task %s: %s\n", t.ID, t.HealthCheck)

	w := m.TaskWorkerMap[t.ID]
	hostPort := getHostPort(t.HostPorts)
	worker := strings.Split(w, ":")
	url := fmt.Sprintf("http://%s:%s%s", worker[0], *hostPort, t.HealthCheck)
	log.Printf("Calling health check for task %s: %s\n", t.ID, url)
	resp, err := http.Get(url)
	if err != nil {
		msg := fmt.Sprintf("Error connecting to %s: %v\n", url, err)
		log.Printf(msg)
		return errors.New(msg)
	}

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("Error health check for task %s did not return 200\n", t.ID)
		log.Println(msg)
		return errors.New(msg)
	}

	log.Printf("Task %s health check response: %v\n", t.ID, resp.StatusCode)

	return nil
}

func (m *Manager) restartTask(t *task.Task) error {
	// Get the worker where the task was running
	w := m.TaskWorkerMap[t.ID]
	t.State = task.Scheduled
	t.RestartCount++
	// Overwite the existing task to ensure it has the current state
	m.TaskDb[t.ID] = t

	te := task.TaskEvent{
		ID:         uuid.New(),
		State:      task.Running,
		Timestatmp: time.Now(),
		Task:       *t,
	}
	data, err := json.Marshal(te)
	if err != nil {
		return fmt.Errorf("Unable to marshal task event object %#+v: %w", te, err)
	}

	url := fmt.Sprintf("http://%s/tasks", w)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		m.Pending.Enqueue(te)
		return fmt.Errorf("Unable to connect to %s: %w", url, err)
	}

	d := json.NewDecoder(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		e := worker.ErrResponse{}
		if err := d.Decode(&e); err != nil {
			return fmt.Errorf("Unable to decode response: %w", err)
		}
		return fmt.Errorf("Response error (%d): %s", e.HTTPStatusCode, e.Message)
	}

	newTask := task.Task{}
	if err = d.Decode(&newTask); err != nil {
		return fmt.Errorf("Unable to decode response: %w", err)
	}
	log.Printf("%#v\n", newTask)

	return nil
}

func (m *Manager) doHealthChecks() {
	for _, t := range m.TaskDb {
		if t.State == task.Running && t.RestartCount < 3 {
			err := m.checkTaskHealth(*t)
			if err != nil {
				if err = m.restartTask(t); err != nil {
					log.Println(err)
				}
			}
		} else if t.State == task.Failed && t.RestartCount < 3 {
			if err := m.restartTask(t); err != nil {
				log.Println(err)
			}
		}
	}
}

func (m *Manager) DoHalthChecks() {
	for {
		log.Println("Performing task health check")
		m.doHealthChecks()
		log.Println("Task health checks completed")
		log.Println("Sleeping for 60 seconds")
		time.Sleep(60 * time.Second)
	}
}
