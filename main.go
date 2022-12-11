package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/Yuya9786/cube/task"
	"github.com/Yuya9786/cube/worker"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

func do() {
	db := make(map[uuid.UUID]*task.Task)
	w := worker.Worker{
		Queue: *queue.New(),
		Db:    db,
	}

	t := task.Task{
		ID:    uuid.New(),
		State: task.Scheduled,
		Image: "strm/helloworld-http",
	}

	// first time the worker will see the task
	fmt.Println("starting task")
	w.AddTask(t)
	result := w.RunTask()
	if result.Error != nil {
		panic(result.Error)
	}

	t.ContainerId = result.ContainerId

	fmt.Printf("task %s is running in container %s\n", t.ID, t.ContainerId)
	fmt.Println("sleepy time")
	time.Sleep(time.Second * 60)

	fmt.Printf("Stopping task %s\n", t.ID)
	t.State = task.Completed
	w.AddTask(t)
	result = w.RunTask()
	if result.Error != nil {
		panic(result.Error)
	}
}

func main() {
	wg := &sync.WaitGroup{}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			do()
			wg.Done()
		}()
	}
	wg.Wait()
}
