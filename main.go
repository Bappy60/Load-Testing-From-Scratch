package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var DB *sql.DB
var RedisClient *redis.Client

// Book struct represents a book
type Book struct {
	ID          int    `json:"id"`
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
	db.SetMaxOpenConns(10) // Adjust as needed
	// Set maximum idle connections
	db.SetMaxIdleConns(10) // Adjust as needed

	// Verify database connection
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	DB = db
	fmt.Println("Connected to database")
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

// createTables creates necessary tables if they don't exist in the database.
func CreateTable() {
	// Define SQL statements to create tables
	createBookTableSQL := `
	CREATE TABLE IF NOT EXISTS books (
		id INT AUTO_INCREMENT PRIMARY KEY,
		title VARCHAR(255) NOT NULL,
		author VARCHAR(255) NOT NULL,
		publication VARCHAR(255) NOT NULL
	)`

	// Execute SQL statements to create tables
	_, err := DB.Exec(createBookTableSQL)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Tables created successfully")
}

func main() {
	// Initialize Echo
	e := echo.New()

	// Load the .env file in the current directory
	godotenv.Load()

	ConnectToDB()
	ConnectToRedis()
	CreateTable()

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

	// Query the database to retrieve the book with the specified ID
	row := DB.QueryRow("SELECT id, title, author FROM books WHERE id = ?", bookID)

	// Scan the row into a Book struct
	var book Book
	err := row.Scan(&book.ID, &book.Title, &book.Author)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Book not found"})
	} else if err != nil {
		log.Fatal(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
	}

	return c.JSON(http.StatusOK, book)
}

func getBooksFromRedis(c echo.Context) error {

	val, err := RedisClient.Get(c.Request().Context(), "books").Result()
	if err != nil {
		log.Fatal(err)
	}

	// Parse JSON string to []Book
	var books []Book
	err = json.Unmarshal([]byte(val), &books)
	if err != nil {
		log.Fatal(err)
	}

	return c.JSON(http.StatusOK, books)
}

func getBooksFromMap(c echo.Context) error {
	// Implement your service layer logic here
	// You can use raw SQL queries to fetch data from the database
	// and store it in a map or any other data structure
	// Here, we just return a placeholder response
	books := []Book{
		{ID: 1, Title: "Book 1", Author: "Author 1"},
		{ID: 2, Title: "Book 2", Author: "Author 2"},
		{ID: 3, Title: "Book 3", Author: "Author 3"},
	}

	return c.JSON(http.StatusOK, books)
}
