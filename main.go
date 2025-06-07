package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/google/uuid"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type Worker struct {
	id   uuid.UUID
	src  chan string
	stop chan bool
	wg   *sync.WaitGroup
}

func (w *Worker) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-w.stop:
				log.Printf("Worker[%s] stopped\n", w.id)
				return
			case msg, ok := <-w.src:
				if !ok {
					log.Printf("Worker[%s]: src closed\n", w.id)
					return
				}
				log.Printf("Worker[%s] received message: %s]\n", w.id, msg)
			}
		}
	}()
}

type WorkerManager struct {
	workers map[uuid.UUID]*Worker
	mutex   *sync.Mutex
}

func NewWorkerManager() *WorkerManager {
	return &WorkerManager{
		workers: make(map[uuid.UUID]*Worker),
		mutex:   &sync.Mutex{},
	}
}

func (wm *WorkerManager) AddWorker(src chan string, wg *sync.WaitGroup) *Worker {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	id := uuid.New()
	worker := &Worker{
		id:   id,
		src:  src,
		stop: make(chan bool),
		wg:   wg,
	}
	wm.workers[id] = worker
	log.Printf("Add worker[%s]\n", worker.id)
	return worker
}

func (wm *WorkerManager) DeleteWorker(id uuid.UUID) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	if worker, ok := wm.workers[id]; ok {
		log.Printf("Delete worker[%s]\n", worker.id)
		worker.stop <- true
		delete(wm.workers, id)
	}
}

func main() {
	wm := NewWorkerManager()
	source := make(chan string)
	wg := &sync.WaitGroup{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		log.Println("\nReceived shutdown signal")
		cancel()
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Worker Pool Manager")
	fmt.Println("Available commands:")
	fmt.Println("  add - add new worker")
	fmt.Println("  delete <id> - delete worker by ID")
	fmt.Println("  list - show all workers")
	fmt.Println("  exit - stop all workers and exit")
	fmt.Println("  or enter message to process")

	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(input, " ", 2)
		cmd := parts[0]

		switch cmd {
		case "add":
			if len(parts) != 2 {
				w := wm.AddWorker(source, wg)
				if w != nil {
					w.Start()
				}
			} else if len(parts) == 2 {
				amount, _ := strconv.Atoi(parts[1])
				for range amount {
					w := wm.AddWorker(source, wg)
					if w != nil {
						w.Start()
					}
				}
			}
		case "delete":
			if len(parts) < 2 {
				log.Println("Please specify worker ID")
				continue
			}
			id, err := uuid.Parse(parts[1])
			if err != nil {
				log.Println("Invalid worker ID format")
				continue
			}
			wm.DeleteWorker(id)

		case "list":
			log.Printf("Active workers (%d):\n", len(wm.workers))
			for id := range wm.workers {
				log.Println(id)
			}

		case "exit":
			cancel()
			return

		default:
			if len(wm.workers) == 0 {
				log.Println("No alive workers. Add at least one")
				continue
			}
			source <- input
		}
	}

	<-ctx.Done()
	for id := range wm.workers {
		wm.DeleteWorker(id)
	}
	close(source)
	wg.Wait()
	log.Println("All workers stopped. Exiting...")
}
