-- Create the database (if not exists)
CREATE DATABASE IF NOT EXISTS conduit;

-- Use the database
USE conduit;

-- Drop the table if it already exists
DROP TABLE IF EXISTS users;

-- Create the users table
CREATE TABLE users (
                       id INT AUTO_INCREMENT PRIMARY KEY,
                       username VARCHAR(50) NOT NULL,
                       email VARCHAR(100) UNIQUE NOT NULL,
                       first_name VARCHAR(50),
                       last_name VARCHAR(50),
                       phone VARCHAR(20),
                       street VARCHAR(100),
                       city VARCHAR(50),
                       state VARCHAR(50),
                       zip_code VARCHAR(10),
                       country VARCHAR(50),
                       status ENUM('active', 'inactive', 'pending'),
                       subscription_type ENUM('free', 'basic', 'premium', 'enterprise'),
                       last_login DATETIME,
                       created_at DATETIME NOT NULL,
                       age INT,
                       notifications BOOLEAN,
                       newsletter BOOLEAN,
                       theme ENUM('light', 'dark', 'auto'),
                       last_updated DATETIME,
                       device_type ENUM('mobile', 'desktop', 'tablet'),
                       browser ENUM('Chrome', 'Firefox', 'Safari', 'Edge')
);

