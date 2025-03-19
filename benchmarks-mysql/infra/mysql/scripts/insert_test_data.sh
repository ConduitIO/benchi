#!/bin/bash

cleanup() {
  echo "Interrupted. Exiting..."
  exit
}

trap cleanup SIGINT

# Number of records per batch
batch_size=20000
batches=10

# MySQL connection details
user="root"
database="conduit"
export MYSQL_PWD="root"

# Function to insert a batch of records
insert_batch() {
  echo "Generating SQL for batch $1..."

  temp_sql_file=$(mktemp)

  echo "INSERT INTO users (username, email, first_name, last_name, phone, street, city, state, zip_code, country, status, subscription_type, last_login, created_at, age, notifications, newsletter, theme, last_updated, device_type, browser) VALUES" > "$temp_sql_file"

  for ((i=1; i<=batch_size; i++)); do
    username="user$(( ($1 - 1) * batch_size + i ))"
    email="${username}@example.com"
    phone="+1$(printf '%09d' $(( RANDOM % 1000000000 )) )"
    street="$(( RANDOM % 1000 )) Main St"
    zip_code="$(printf '%05d' $(( RANDOM % 100000 )) )"

    echo -n "('$username', '$email', 'John', 'Doe', '$phone', '$street', 'New York', 'NY', '$zip_code', 'USA', 'active', 'premium', NOW(), NOW(), $(( 18 + RANDOM % 50 )), true, false, 'dark', NOW(), 'desktop', 'Chrome')" >> "$temp_sql_file"

    if [ "$i" -lt "$batch_size" ]; then
      echo -n "," >> "$temp_sql_file"
    else
      echo ";" >> "$temp_sql_file"
    fi
  done

  echo "Inserting batch $1..."
  mysql -u"$user" "$database" < "$temp_sql_file"

  rm "$temp_sql_file"
}

# Record start time
start_time=$(date +%s)

# Insert records in multiple batches
for ((j=1; j<=batches; j++)); do
  insert_batch "$j"
done

# Record end time
end_time=$(date +%s)

# Calculate duration
duration=$((end_time - start_time))

echo "Inserted $((batch_size * batches)) users in $duration seconds."
