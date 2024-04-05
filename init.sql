-- Create the database and the table
CREATE DATABASE IF NOT EXISTS demodb2;
USE demodb2;
CREATE TABLE IF NOT EXISTS books (
    id INT AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    author VARCHAR(255) NOT NULL,
    publication VARCHAR(255) NOT NULL
);

-- Function to generate random strings
DELIMITER $$
CREATE FUNCTION random_string(length INT)
RETURNS VARCHAR(255)
BEGIN
    DECLARE chars_str VARCHAR(52) DEFAULT 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ';
    DECLARE return_str VARCHAR(255) DEFAULT '';
    DECLARE i INT DEFAULT 0;
    WHILE i < length DO
        SET return_str = CONCAT(return_str, SUBSTRING(chars_str, FLOOR(RAND() * 52) + 1, 1));
        SET i = i + 1;
    END WHILE;
    RETURN return_str;
END$$
DELIMITER ;

-- Procedure to insert dummy records
DELIMITER $$
CREATE PROCEDURE insert_dummy_books(num_records INT)
BEGIN
    DECLARE i INT DEFAULT 0;
    WHILE i < num_records DO
        INSERT INTO books (title, author, publication)
        VALUES (CONCAT('Book ', i+1), random_string(10), random_string(15));
        SET i = i + 1;
    END WHILE;
END$$
DELIMITER ;

-- Call the procedure to insert 100,000 records
CALL insert_dummy_books(100000);