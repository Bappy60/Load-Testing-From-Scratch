package main

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// Define the error counter as an int64 global variable
var errCounter int64

// Retrieve the current value of the error counter
func getErrorCounter() int64 {
	return errCounter
}

// Increment the error counter
func incrementErrorCounter() {
	errCounter++
}

// Worker is a struct that represents a concurrent worker
type Worker struct {
	id               int          // worker id
	url              string       // url to make requests to
	rpsForEachWorker int          // Requests per second for each worker
	client           *http.Client // HTTP client to use
}

// Result is a struct that holds the result of a request
type Result struct {
	workerID int           // worker id
	status   int           // status code
	latency  time.Duration // latency
	err      error         // error if any
}

// NewWorker creates a new worker with the given parameters
func NewWorker(id int, url string, rpsForEachWorker int, client *http.Client) *Worker {
	return &Worker{
		id:               id,
		url:              url,
		rpsForEachWorker: rpsForEachWorker,
		client:           client,
	}
}

// Run runs the worker and sends the results to the given channel
func (w *Worker) Run(results chan<- Result, duration time.Duration) {
	defer func() {
		// handle panic gracefully
		if r := recover(); r != nil {
			incrementErrorCounter()
			fmt.Println("Worker", w.id, "panicked:", r)
		}
	}()

	// Calculate request rate per second for each worker
	requestRatePerSecond := w.rpsForEachWorker / int(duration)
	// Calculate sleep duration between each request
	sleepDuration := time.Second / time.Duration(requestRatePerSecond)

	// Loop for the duration of d seconds
	for j := 0; j < int(duration); j++ {
		// Loop to make requests at the desired rate
		for i := 0; i < requestRatePerSecond; i++ {
			// Make a GET request and measure the latency
			start := time.Now()
			resp, err := w.client.Get(w.url)
			latency := time.Since(start)

			// Send the result to the channel
			result := Result{w.id, 0, latency, err}
			if err != nil {
				fmt.Println(err)
				incrementErrorCounter()
			} else {
				// Close the response body immediately after use
				defer resp.Body.Close()
				result.status = resp.StatusCode
			}

			// Send the result to the channel
			results <- result

			// Sleep for the calculated duration before making the next request
			time.Sleep(sleepDuration)
		}
	}
}

// Define a struct to store the status code metrics
type StatusCodeMetrics struct {
	Count      int // number of requests with this status code
	MinLatency time.Duration
	MaxLatency time.Duration
	SumLatency time.Duration // sum of latencies for this status code
}

// LoadTestMetrics holds the load test metrics
type LoadTestMetrics struct {
	TotalRequests     int                       `json:"total_requests"`
	AverageLatency    time.Duration             `json:"average_latency"`
	RequestsPerSecond int                       `json:"requests_per_second"`
	MinLatency        time.Duration             `json:"min_latency"`
	MaxLatency        time.Duration             `json:"max_latency"`
	ErrorRate         float64                   `json:"error_rate"`
	StatusMetrics     map[int]*StatusCodeMetrics `json:"status_metrics"`
}

// LoadTestHandler handles the load testing
func LoadTestHandler(c echo.Context) error {
	// Parse query parameters
	url := c.QueryParam("url")
	rps, err := strconv.Atoi(c.QueryParam("rps"))
	if err != nil {
		return err
	}
	duration, err := strconv.Atoi(c.QueryParam("duration"))
	if err != nil {
		return err
	}

	// Calculate total number of requests needed
	totalRequests := rps * duration

	// Determine the number of workers based on the number of CPUs
	workers := runtime.NumCPU()
	fmt.Printf("Num of workers: %d\n", workers)

	// Calculate the number of requests per worker
	requestsPerWorker := totalRequests / workers

	// Create a wait group for workers
	wg := &sync.WaitGroup{}
	wg.Add(workers)

	// Create a channel for results
	results := make(chan Result, totalRequests)

	// Calculate the request timeout
	timeout := time.Duration(duration+3) * time.Second

	// Create an HTTP client with the calculated timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Create a map to store status code metrics
	statusMetrics := make(map[int]*StatusCodeMetrics)

	// Create and run workers
	for i := 0; i < workers; i++ {
		worker := NewWorker(i, url, requestsPerWorker, client)
		go func(worker *Worker) {
			worker.Run(results, time.Duration(duration))
			wg.Done()
		}(worker)
	}

	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect and print metrics
	var minLatency, maxLatency, sumLatency int64
	minLatency = 1<<63 - 1 // max int64 value

	// Iterate over the results
	for result := range results {
		// if result.err != nil {
		// 	totalErrors++
		// }
		sumLatency += int64(result.latency)
		if result.latency < time.Duration(minLatency) {
			minLatency = int64(result.latency)
		}
		if result.latency > time.Duration(maxLatency) {
			maxLatency = int64(result.latency)
		}

		// Update status code metrics
		if _, ok := statusMetrics[result.status]; !ok {
			statusMetrics[result.status] = &StatusCodeMetrics{
				Count:      0,
				MinLatency: 1<<63 - 1,
				MaxLatency: 0,
				SumLatency: 0,
			}
		}
		statusMetrics[result.status].Count++
		statusMetrics[result.status].SumLatency += result.latency
		if result.latency < time.Duration(statusMetrics[result.status].MinLatency) {
			statusMetrics[result.status].MinLatency = result.latency
		}
		if result.latency > time.Duration(statusMetrics[result.status].MaxLatency) {
			statusMetrics[result.status].MaxLatency = result.latency
		}
	}
	avgLatency := float64(sumLatency) / float64(totalRequests)

	// Create the LoadTestMetrics struct
	loadTestMetrics := LoadTestMetrics{
		TotalRequests:     totalRequests,
		AverageLatency:    time.Duration(avgLatency),
		RequestsPerSecond: rps,
		MinLatency:        time.Duration(minLatency),
		MaxLatency:        time.Duration(maxLatency),
		ErrorRate:         float64(getErrorCounter()) / float64(totalRequests) * 100,
		StatusMetrics:     statusMetrics,
	}

	return c.JSON(http.StatusOK, loadTestMetrics)
}

// main function
func main() {
	// Initialize Echo
	e := echo.New()

	// Register handler for load testing
	e.GET("/loadtest", LoadTestHandler)

	// Start server
	e.Logger.Fatal(e.Start(":9012"))
}

