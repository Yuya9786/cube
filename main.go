package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/Yuya9786/cube/task"
	"github.com/Yuya9786/cube/worker"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

func runTasks(w *worker.Worker) {
	for {
		if w.Queue.Len() != 0 {
			result := w.RunTask()
			if result.Error != nil {
				log.Printf("Error running task: %v\n", result.Error)
			}
		} else {
			log.Printf("No tasks to process currently\n")
		}
		log.Printf("Sleeping 10 seconds\n")
		time.Sleep(10 * time.Second)
	}
}

func main() {
	host := os.Getenv("CUBE_HOST")
	port, _ := strconv.Atoi(os.Getenv("CUBE_PORT"))

	fmt.Println("Strting Cube worker")

	w := worker.Worker{
		Queue: *queue.New(),
		Db:    make(map[uuid.UUID]*task.Task),
	}

	api := worker.Api{
		Address: host,
		Port:    port,
		Worker:  &w,
	}

	go runTasks(&w)
	go w.CollectState()

	log.Printf("Starting API %v:%v\n", api.Address, api.Port)
	api.Start()
}
