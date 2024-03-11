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

// Book struct represents a book
type Book struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Publication string `json:"publication"`
}

// MySQL configuration
var (
	DBUsername = os.Getenv("DBUsername")
	DBPassword = os.Getenv("DBPassword")
	DBHost     = os.Getenv("DBHost")
	DBPort     = os.Getenv("DBPort")
	DBName     = os.Getenv("DBName")
)

// Redis configuration
const (
	RedisHost = "localhost"
	RedisPort = "6379"
)

var DB *sql.DB

func ConnectToDB() {
	// connect with database
	DB, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", DBUsername, DBPassword, DBHost, DBPort, DBName))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Database Connected..")
	defer DB.Close()

}

func main() {
	// Initialize Echo
	e := echo.New()

	// Load the .env file in the current directory
	godotenv.Load()

	ConnectToDB()
	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Routes
	e.GET("/books/db", getBooksFromDB)
	e.GET("/books/redis", getBooksFromRedis)
	e.GET("/books/service", getBooksFromMap)

	// Start server
	e.Logger.Fatal(e.Start(":9011"))
}

func getBooksFromDB(c echo.Context) error {

	rows, err := DB.Query("SELECT id, title, author FROM books")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		err := rows.Scan(&book.ID, &book.Title, &book.Author)
		if err != nil {
			log.Fatal(err)
		}
		books = append(books, book)
	}

	return c.JSON(http.StatusOK, books)
}

func getBooksFromRedis(c echo.Context) error {
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", RedisHost, RedisPort),
	})
	defer rdb.Close()

	val, err := rdb.Get(c.Request().Context(), "books").Result()
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
