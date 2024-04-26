- DBOS
  - Message System: A message table with sender, receiver, payload
    - Partitioned on receiver
    - Sending a message: SQL insert
    - Reading a message: SQL query followed by a delete
  - File System: A File table with owner, permissions, partitioning
    - A Directory table
    - A Blocks table
    - Partition Blocks on owner: Linux FS behavior
    - Partition Blocks on block ID: Lustre behavior
- considerations
  - "tombstone" data elements (locally?) with a bool rather than deleting, add some mechanism to mark diseased nodes, priority
  - only syndicate messages to people with permissions

https://github.com/mattn/go-sqlite3/blob/master/_example/simple/simple.go
https://github.com/jervisfm/resqlite/tree/master
