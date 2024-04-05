package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Define the error counter as an int64 global variable
var (
	errCounter  int64
	errCounterM sync.Mutex // Mutex for error counter
)

// Retrieve the current value of the error counter
func getErrorCounter() int64 {
	errCounterM.Lock()
	defer errCounterM.Unlock()
	return errCounter
}

// Increment the error counter
func incrementErrorCounter() {
	errCounterM.Lock()
	defer errCounterM.Unlock()
	errCounter++
}

// Worker is a struct that represents a concurrent worker
type Worker struct {
	id     int
	url    string
	client *http.Client
	mutex  sync.Mutex // Mutex for synchronizing access to results channel
}

// Result is a struct that holds the result of a request
type Result struct {
	workerID int           // worker id
	status   int           // status code
	latency  time.Duration // latency
	err      error         // error if any
}

// NewWorker creates a new worker with the given parameters
// NewWorker creates a new worker with the given parameters
func NewWorker(id int, url string, client *http.Client) *Worker {
	return &Worker{
		id:     id,
		url:    url,
		client: client,
		mutex:  sync.Mutex{}, // Initialize the mutex
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

	// Protect access to the results channel with a mutex
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// Send the result to the channel
	results <- result
}

// Write load test metrics to a CSV file
func writeMetricsToCSV(url string, metrics LoadTestMetrics) error {
	// Open the CSV file for writing in append mode
	file, err := os.OpenFile("metrics.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create a CSV writer
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Convert load test metrics to a slice of strings
	var record []string
	record = append(record, url)
	record = append(record, strconv.Itoa(metrics.TotalRequests))
	record = append(record, metrics.AverageLatency)
	record = append(record, strconv.Itoa(metrics.RequestsPerSecond))
	record = append(record, metrics.MinLatency)
	record = append(record, metrics.MaxLatency)
	record = append(record, fmt.Sprintf("%.2f", metrics.ErrorRate))

	// Convert ResponseStatusCodeMetrics to a slice of strings
	for statusCode, statusMetrics := range metrics.ResStatusMetrics {
		record = append(record, strconv.Itoa(statusCode))
		record = append(record, strconv.Itoa(statusMetrics.Count))
		record = append(record, statusMetrics.MinLatency)
		record = append(record, statusMetrics.MaxLatency)
		record = append(record, statusMetrics.AvgLatency)
	}

	// Write metrics to the CSV file
	if err := writer.Write(record); err != nil {
		return err
	}

	return nil
}

// Define a struct to store the status code metrics
type StatusCodeMetrics struct {
	Count      int // number of requests with this status code
	MinLatency time.Duration
	MaxLatency time.Duration
	SumLatency time.Duration // sum of latencies for this status code
}
type ResponseStatusCodeMetrics struct {
	Count      int // number of requests with this status code
	MinLatency string
	MaxLatency string
	AvgLatency string // average of latencies for this status code
}

// LoadTestMetrics holds the load test metrics
type LoadTestMetrics struct {
	TotalRequests     int                                `json:"total_requests"`
	AverageLatency    string                             `json:"average_latency"`
	RequestsPerSecond int                                `json:"requests_per_second"`
	MinLatency        string                             `json:"min_latency"`
	MaxLatency        string                             `json:"max_latency"`
	ErrorRate         float64                            `json:"error_rate"`
	ResStatusMetrics  map[int]*ResponseStatusCodeMetrics `json:"status_metrics"` // map of status code metrics
	P50               string                             `json:"p50"`
	P90               string                             `json:"p90"`
	P95               string                             `json:"p95"`
	P99               string                             `json:"p99"`
}

// LoadTestHandler handles the load testing
func LoadTestHandler(w http.ResponseWriter, r *http.Request) {
	// Reset error counter before starting a new test
	errCounterM.Lock()
	errCounter = 0
	errCounterM.Unlock()

	// Parse query parameters
	url := r.URL.Query().Get("url")
	rps, err := strconv.Atoi(r.URL.Query().Get("rps"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	duration, err := strconv.Atoi(r.URL.Query().Get("duration"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Calculate total number of requests needed
	totalRequests := rps * duration

	// Create a wait group for workers
	wg := &sync.WaitGroup{}

	// Create a channel for results
	results := make(chan Result, totalRequests)

	// Calculate the request timeout
	timeout := time.Second * time.Duration(duration)

	// Create an HTTP client with the calculated timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Create a map to store status code metrics
	statusMetrics := make(map[int]*StatusCodeMetrics)
	statusMetricsM := sync.Mutex{} // Mutex for statusMetrics

	// Slice to store latencies of all requests
	var latencies []time.Duration

	for i := 0; i < duration; i++ {
		// Create and run workers
		for i := 0; i < rps; i++ {
			worker := NewWorker(i, url, client)
			wg.Add(1)
			go func(worker *Worker) {
				worker.Run(results, time.Duration(duration))
				wg.Done()
			}(worker)
		}
		time.Sleep(time.Second)
	}
	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect and print metrics
	var minLatency, maxLatency time.Duration
	// Declare a variable to store the sum of latencies
	var sumLatency time.Duration
	minLatency = time.Duration(math.MaxInt64)
	totalErrors := getErrorCounter()

	// Iterate over the results
	for result := range results {
		if result.err != nil {
			incrementErrorCounter()
			continue
		}
		// Add the latency to the sum
		sumLatency += result.latency

		// Add latency to slice
		latencies = append(latencies, result.latency)

		if result.latency < minLatency {
			minLatency = result.latency
		}
		if result.latency > maxLatency {
			maxLatency = result.latency
		}

		// Update status code metrics
		statusMetricsM.Lock()
		if _, ok := statusMetrics[result.status]; !ok {
			statusMetrics[result.status] = &StatusCodeMetrics{
				Count:      0,
				MinLatency: time.Duration(math.MaxInt64),
				MaxLatency: 0,
				SumLatency: 0,
			}
		}
		statusMetrics[result.status].Count++
		statusMetrics[result.status].SumLatency += result.latency
		if result.latency < statusMetrics[result.status].MinLatency {
			statusMetrics[result.status].MinLatency = result.latency
		}
		if result.latency > statusMetrics[result.status].MaxLatency {
			statusMetrics[result.status].MaxLatency = result.latency
		}
		statusMetricsM.Unlock()
	}

	// Calculate average latency
	avgLatency := time.Duration(0)
	if totalRequests > 0 {
		avgLatency = sumLatency / time.Duration(totalRequests)
	}

	// Calculate percentiles
	p50 := calculatePercentile(latencies, 50)
	p90 := calculatePercentile(latencies, 90)
	p95 := calculatePercentile(latencies, 95)
	p99 := calculatePercentile(latencies, 99)

	// Calculate error rate
	errorRate := float64(totalErrors) / float64(totalRequests) * 100

	resStatusMetrics := make(map[int]*ResponseStatusCodeMetrics)
	for status, metrics := range statusMetrics {
		resStatusMetrics[status] = &ResponseStatusCodeMetrics{
			Count:      metrics.Count,
			MinLatency: metrics.MinLatency.String(),
			MaxLatency: metrics.MaxLatency.String(),
			AvgLatency: (metrics.SumLatency / time.Duration(metrics.Count)).String(),
		}
	}
	// Create the LoadTestMetrics struct
	loadTestMetrics := LoadTestMetrics{
		TotalRequests:     totalRequests,
		AverageLatency:    avgLatency.String(),
		RequestsPerSecond: rps,
		MinLatency:        minLatency.String(),
		MaxLatency:        maxLatency.String(),
		ErrorRate:         errorRate,
		ResStatusMetrics:  resStatusMetrics,
		P50:               p50.String(),
		P90:               p90.String(),
		P95:               p95.String(),
		P99:               p99.String(),
	}
	// Write load test metrics to CSV file
	if err := writeMetricsToCSV(url, loadTestMetrics); err != nil {
		fmt.Println("Error writing metrics to CSV:", err)
	}

	// Return load test metrics as JSON
	w.Header().Set("Content-Type", "application/json")
	// Marshal loadTestMetrics to JSON
	responseJSON, err := json.Marshal(loadTestMetrics)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set Content-Type header
	w.Header().Set("Content-Type", "application/json")

	// Write JSON response
	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON)
}

// calculatePercentile calculates the nth percentile of the given data
func calculatePercentile(data []time.Duration, percentile int) time.Duration {
	if len(data) == 0 {
		return 0
	}
	// Sort the data in ascending order
	sort.Slice(data, func(i, j int) bool {
		return data[i] < data[j]
	})
	// Determine the index position for the percentile
	index := float64(percentile) / 100 * float64(len(data)-1)
	// Check if the index is an integer
	if index == float64(int(index)) {
		// If the index is an integer, return the value at the index
		return data[int(index)]
	}
	// If the index is not an integer, interpolate between the two nearest values
	lowerIndex := int(math.Floor(index))
	upperIndex := int(math.Ceil(index))
	lowerValue := data[lowerIndex]
	upperValue := data[upperIndex]
	// Interpolate between the two nearest values
	// Convert the difference between index and lowerIndex to time.Duration for multiplication
	indexOffset := time.Duration(index - float64(lowerIndex))
	// Interpolate between the two nearest values
	interpolatedValue := lowerValue + (upperValue-lowerValue)*indexOffset
	return interpolatedValue
}

func main() {
	http.HandleFunc("/loadtest", LoadTestHandler)

	server := &http.Server{Addr: ":9012"}

	// Graceful shutdown handling
	go func() {
		// Monitor for shutdown signal (e.g., SIGINT, SIGTERM)
		quit := make(chan os.Signal,1)
		signal.Notify(quit, os.Interrupt, os.Interrupt, syscall.SIGTERM)
		<-quit
		log.Println("Shutdown signal received, initiating graceful shutdown...")

		// Set a timeout for graceful shutdown (optional)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt to gracefully shut down the server within the timeout
		err := server.Shutdown(ctx)
		if err != nil {
			log.Printf("Error during graceful shutdown: %v\n", err)
		}
		log.Println("Server stopped.")
	}()

	log.Fatal(server.ListenAndServe()) // Start the server with error handling
}
