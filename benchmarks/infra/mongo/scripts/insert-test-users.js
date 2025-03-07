// create_users_batch.js

// Create arrays of possible values for random selection
const statuses = ['active', 'inactive', 'pending'];
const subscriptionTypes = ['free', 'basic', 'premium', 'enterprise'];
const themes = ['light', 'dark', 'auto'];
const devices = ['mobile', 'desktop', 'tablet'];
const browsers = ['Chrome', 'Firefox', 'Safari', 'Edge'];


// Total number of documents and batch size
const totalDocs = 2_000_000;
const batchSize = 20_000;
let batch = [];

// Loop through and generate documents
for (let i = 0; i < totalDocs; i++) {
  const doc = {
    userId: new ObjectId(),
    username: 'user' + i,
    email: `user${i}@example.com`,
    firstName: ['John', 'Jane', 'Bob', 'Alice'][Math.floor(Math.random() * 4)],
    lastName: ['Smith', 'Johnson', 'Brown', 'Davis'][Math.floor(Math.random() * 4)],
    phone: `+1${Math.floor(Math.random() * 1000000000)}`,
    address: {
      street: `${Math.floor(Math.random() * 1000)} Main St`,
      city: ['New York', 'Los Angeles', 'Chicago', 'Houston'][Math.floor(Math.random() * 4)],
      state: ['NY', 'CA', 'IL', 'TX'][Math.floor(Math.random() * 4)],
      zipCode: Math.floor(Math.random() * 90000 + 10000).toString(),
      country: 'USA'
    },
    status: statuses[Math.floor(Math.random() * statuses.length)],
    subscriptionType: subscriptionTypes[Math.floor(Math.random() * subscriptionTypes.length)],
    age: Math.floor(Math.random() * 62) + 18,
    preferences: {
      notifications: Math.random() > 0.5,
      newsletter: Math.random() > 0.5,
      theme: themes[Math.floor(Math.random() * themes.length)]
    },
    metadata: {
      deviceType: devices[Math.floor(Math.random() * devices.length)],
      browser: browsers[Math.floor(Math.random() * browsers.length)]
    }
  };

  // Add the document to the current batch
  batch.push(doc);

  // If the batch is full, insert it and reset the batch array
  if (batch.length === batchSize) {
    db.users.insertMany(batch);
    print(`Inserted batch ${Math.floor(i / batchSize) + 1}: ${batch.length} documents`);
    batch = [];
  }
}

// Insert any remaining documents in the final (incomplete) batch
if (batch.length > 0) {
  db.users.insertMany(batch);
  print(`Inserted final batch: ${batch.length} documents`);
}

print(`Finished inserting ${totalDocs} documents.`);
print('\nSample document:');
printjson(db.users.findOne());
