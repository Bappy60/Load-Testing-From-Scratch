CREATE DATABASE IF NOT EXISTS demodb2;
USE demodb2;

CREATE TABLE IF NOT EXISTS books (
    id INT AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    author VARCHAR(255) NOT NULL,
    publication VARCHAR(255) NOT NULL
);

INSERT INTO books (title, author, publication) VALUES
('Book 1', 'Author 1', 'Publisher 1'),
('Book 2', 'Author 2', 'Publisher 2'),
('Book 3', 'Author 3', 'Publisher 3'),
('Book 4', 'Author 4', 'Publisher 4'),
('Book 5', 'Author 5', 'Publisher 5'),
('Book 6', 'Author 6', 'Publisher 6'),
('Book 7', 'Author 7', 'Publisher 7'),
('Book 8', 'Author 8', 'Publisher 8'),
('Book 9', 'Author 9', 'Publisher 9'),
('Book 10', 'Author 10', 'Publisher 10');
