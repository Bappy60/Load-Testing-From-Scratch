package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

var DB *sql.DB
var RedisClient *redis.Client
var ctx = context.Background()

type Book struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Publication string `json:"publication"`
}

var bookCacheMap = make(map[int64]interface{})

func ConnectToDBWithRetry() {
	// Read database credentials from environment variables
	dbUser := os.Getenv("DBUsername")
	dbPass := os.Getenv("DBPassword")
	dbHost := os.Getenv("DBHost")
	dbPort := os.Getenv("DBPort")
	dbName := os.Getenv("DBName")

	// Create data source name
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", dbUser, dbPass, dbHost, dbPort, dbName)
	// Set the maximum number of retries and retry interval
	maxRetries := 5
	retryInterval := 3 * time.Second

	for i := 1; i <= maxRetries; i++ {
		// Add a delay before each retry
		time.Sleep(retryInterval)

		// Attempt to connect to the database
		db, err := sql.Open("mysql", dsn)
		if err == nil {
			// Set maximum open connections
			db.SetMaxOpenConns(20)
			// Set maximum idle connections
			db.SetMaxIdleConns(20)

			// Verify database connection
			err = db.Ping()
			if err != nil {
				log.Fatal(err)
			}
			// Connection successful
			fmt.Println("Database Connected")
			DB = db
			return
		}
		// Log the error, and retry
		fmt.Printf("Error connecting to DB: %v. Retrying...\n", err)

	}
	fmt.Printf("Failed to connect DB")

}

func ConnectToRedis() {
	redisHost := os.Getenv("RedisHost")
	redisPort := os.Getenv("RedisPort")
	redisPassword := os.Getenv("RedisPassword")

	redisOptions := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPassword,
		DB:       0,
	}

	client := redis.NewClient(redisOptions)

	_, err := client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	RedisClient = client
	fmt.Println("Connected to Redis")
}

func main() {
	godotenv.Load()

	ConnectToRedis()
	ConnectToDBWithRetry()

	http.HandleFunc("/book/db", getBookFromDB)
	http.HandleFunc("/book/redis", getBooksFromRedis)
	http.HandleFunc("/book/service", getBooksFromMap)

	log.Fatal(http.ListenAndServe(":9011", nil))
}

func getBookFromDB(w http.ResponseWriter, r *http.Request) {
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		http.Error(w, "Book ID is required", http.StatusBadRequest)
		return
	}

	parsedID, err := strconv.ParseInt(bookID, 0, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	row := DB.QueryRow("SELECT id, title, author, publication FROM books WHERE id = ?", parsedID)

	var book Book
	err = row.Scan(&book.ID, &book.Title, &book.Author, &book.Publication)
	if err == sql.ErrNoRows {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(book)
}

func getBooksFromRedis(w http.ResponseWriter, r *http.Request) {
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		http.Error(w, "Book ID is required", http.StatusBadRequest)
		return
	}

	parsedID, err := strconv.ParseInt(bookID, 0, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cacheKey := fmt.Sprintf("%d", parsedID)
	if book, err := CheckInCache(cacheKey); err == nil {
		json.NewEncoder(w).Encode(book)
		return
	}

	row := DB.QueryRow("SELECT id, title, author, publication FROM books WHERE id = ?", bookID)

	var book Book
	err = row.Scan(&book.ID, &book.Title, &book.Author, &book.Publication)
	if err == sql.ErrNoRows {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := SetInCache(book, cacheKey); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(book)
}

func getBooksFromMap(w http.ResponseWriter, r *http.Request) {
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		http.Error(w, "Book ID is required", http.StatusBadRequest)
		return
	}

	parsedID, err := strconv.ParseInt(bookID, 0, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if cachedBook, ok := bookCacheMap[parsedID]; ok {
		json.NewEncoder(w).Encode(cachedBook.(Book))
		return
	}

	row := DB.QueryRow("SELECT id, title, author, publication FROM books WHERE id = ?", bookID)

	var book Book
	err = row.Scan(&book.ID, &book.Title, &book.Author, &book.Publication)
	if err == sql.ErrNoRows {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	bookCacheMap[book.ID] = book

	json.NewEncoder(w).Encode(book)
}

func CheckInCache(cacheKey string) (book Book, err error) {
	cachedData, err := RedisClient.Get(ctx, cacheKey).Result()
	if err == nil {
		var book Book
		err := json.Unmarshal([]byte(cachedData), &book)
		if err != nil {
			return Book{}, err
		}
		return book, nil
	}
	return Book{}, err
}

func SetInCache(book Book, cacheKey string) error {
	jsonData, err := json.Marshal(book)
	if err != nil {
		return err
	}
	if _, err := RedisClient.Set(ctx, cacheKey, jsonData, time.Hour).Result(); err != nil {
		return err
	}
	return nil
}
