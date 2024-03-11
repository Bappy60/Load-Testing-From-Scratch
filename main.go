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
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var DB *sql.DB
var RedisClient *redis.Client
var ctx = context.Background()

// Define a map to cache books in service layer
var bookCacheMap sync.Map

// Book struct represents a book
type Book struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Publication string `json:"publication"`
}

// ConnectToDB establishes a connection to the MySQL database.
func ConnectToDB() {
	// Read database credentials from environment variables
	dbUser := os.Getenv("DBUsername")
	dbPass := os.Getenv("DBPassword")
	dbHost := os.Getenv("DBHost")
	dbPort := os.Getenv("DBPort")
	dbName := os.Getenv("DBName")

	// Create data source name
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPass, dbHost, dbPort, dbName)

	// Open database connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}

	// Set maximum open connections
	db.SetMaxOpenConns(10)
	// Set maximum idle connections
	db.SetMaxIdleConns(10)

	// Verify database connection
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	DB = db
	fmt.Println("Connected to database")
}
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
			db.SetMaxOpenConns(10)
			// Set maximum idle connections
			db.SetMaxIdleConns(10)

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

// ConnectToRedis establishes a connection to the Redis database.
func ConnectToRedis() {
	redisHost := os.Getenv("RedisHost")
	redisPort := os.Getenv("RedisPort")
	redisPassword := os.Getenv("RedisPassword")

	// Create Redis connection options
	redisOptions := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPassword,
		DB:       0, // Use default DB
	}

	// Create Redis client
	client := redis.NewClient(redisOptions)

	// Test connection
	_, err := client.Ping(client.Context()).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	RedisClient = client
	fmt.Println("Connected to Redis")
}

func main() {
	// Initialize Echo
	e := echo.New()

	// Load the .env file in the current directory
	godotenv.Load()

	ConnectToRedis()
	ConnectToDBWithRetry()
	PopulateBookCacheMap()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Group routes under the prefix "app/book/"
	bookGroup := e.Group("/app/book")
	// Routes
	bookGroup.GET("/db", getBookFromDB)
	bookGroup.GET("/redis", getBooksFromRedis)
	bookGroup.GET("/service", getBooksFromMap)

	// Start server
	e.Logger.Fatal(e.Start(":9011"))
}

func getBookFromDB(c echo.Context) error {
	// Retrieve book ID from query parameter
	bookID := c.QueryParam("id")
	if bookID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Book ID is required"})
	}

	parsedId, err := strconv.ParseInt(bookID, 0, 0)
	if err != nil && bookID != "" {
		return err
	}
	// Query the database to retrieve the book with the specified ID
	row := DB.QueryRow("SELECT id, title, author,publication FROM books WHERE id = ?", parsedId)

	// Scan the row into a Book struct
	var book Book
	err = row.Scan(&book.ID, &book.Title, &book.Author, &book.Publication)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Book not found"})
	} else if err != nil {
		log.Fatal(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
	}

	return c.JSON(http.StatusOK, book)
}

func getBooksFromRedis(c echo.Context) error {
	// Retrieve book ID from query parameter
	bookID := c.QueryParam("id")
	if bookID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Book ID is required"})
	}
	parsedId, err := strconv.ParseInt(bookID, 0, 0)
	if err != nil && bookID != "" {
		return err
	}

	cacheKey := fmt.Sprintf("%d", parsedId)
	if book, err := CheckInCache(cacheKey); err == nil {
		return c.JSON(http.StatusOK, book)
	}

	// Query the database to retrieve the book with the specified ID
	row := DB.QueryRow("SELECT id, title, author,publication FROM books WHERE id = ?", bookID)

	// Scan the row into a Book struct
	var book Book
	err = row.Scan(&book.ID, &book.Title, &book.Author, &book.Publication)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Book not found"})
	} else if err != nil {
		log.Fatal(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
	}

	if err := SetInCache(book, cacheKey); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, book)
}

func getBooksFromMap(c echo.Context) error {
	// Retrieve book ID from query parameter
	bookID := c.QueryParam("id")
	if bookID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Book ID is required"})
	}
	parsedId, err := strconv.ParseInt(bookID, 0, 0)
	if err != nil && bookID != "" {
		return err
	}

	if cachedBook, ok := bookCacheMap.Load(parsedId); ok {
		return c.JSON(http.StatusOK, cachedBook.(Book))
	}

	// Query the database to retrieve the book with the specified ID
	row := DB.QueryRow("SELECT id, title, author,publication FROM books WHERE id = ?", bookID)

	// Scan the row into a Book struct
	var book Book
	err = row.Scan(&book.ID, &book.Title, &book.Author, &book.Publication)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Book not found"})
	} else if err != nil {
		log.Fatal(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
	}

	bookCacheMap.Store(book.ID, book)

	return c.JSON(http.StatusOK, book)
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

// Function to populate the map with books from the database
func PopulateBookCacheMap() {
	// Query the database to retrieve all books
	rows, err := DB.Query("SELECT * FROM books")
	if err != nil {
		log.Fatal(err)
		return
	}
	defer rows.Close()

	// Iterate over the rows
	for rows.Next() {
		var book Book
		// Scan the row into the Book struct
		err := rows.Scan(&book.ID, &book.Title, &book.Author, &book.Publication)
		if err != nil {
			log.Fatal(err)
			continue
		}
		// Store the book in the cache map
		bookCacheMap.Store(book.ID, book)
	}
	// Check for errors during iteration
	if err := rows.Err(); err != nil {
		log.Fatal(err)
		return
	}
}
