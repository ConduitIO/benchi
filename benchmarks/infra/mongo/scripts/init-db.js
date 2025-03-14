// Connect to the primary node
const conn = new Mongo("mongodb://mongo1:30001,mongo2:30002,mongo3:30003/test?replicaSet=test-replica-set");
const testDB = conn.getDB('test');  // Use a different variable name

// Ensure the 'users' collection exists
testDB.createCollection('users', { capped: false });

// Insert the document
const doc = {
    userId: new ObjectId(),
    username: 'user0',
    email: 'user0@example.com'
};

const insertResult = testDB.users.insertOne(doc);
print("Inserted document with _id:", insertResult.insertedId);

// Delete the document we just inserted (using the _id automatically added by MongoDB)
const deleteResult = testDB.users.deleteOne({ _id: insertResult.insertedId });
print("Deleted documents count:", deleteResult.deletedCount);
