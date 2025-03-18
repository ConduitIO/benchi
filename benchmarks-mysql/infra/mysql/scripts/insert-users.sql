USE conduit;

DELIMITER $$

DROP PROCEDURE IF EXISTS generate_users$$

CREATE PROCEDURE generate_users(IN num INT)
BEGIN
  DECLARE i INT DEFAULT 0;
  WHILE i < num DO
    INSERT INTO users (
      username, email, first_name, last_name, phone, street, city, state, zip_code, country,
      status, subscription_type, last_login, created_at, age, notifications, newsletter,
      theme, last_updated, device_type, browser
    )
    VALUES (
      CONCAT('user', i),
      CONCAT('user', i, '@example.com'),
      'John',
      'Doe',
      CONCAT('+1', LPAD(FLOOR(RAND() * 1000000000), 9, '0')),
      CONCAT(FLOOR(RAND() * 1000), ' Main St'),
      'New York',
      'NY',
      LPAD(FLOOR(RAND() * 100000), 5, '0'),
      'USA',
      'active',
      'premium',
      NOW(),
      NOW(),
      FLOOR(RAND() * 62) + 18,
      TRUE,
      FALSE,
      'dark',
      NOW(),
      'desktop',
      'Chrome'
    );
    SET i = i + 1;
END WHILE;
END$$

DELIMITER ;

CALL generate_users(500000);
