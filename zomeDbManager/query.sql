-- name: AddUser :execresult
INSERT INTO Users (id, username, password_hash)
VALUES (?, ?, ?);

-- name: GetUser :one
SELECT * FROM Users WHERE id = ?;

-- name: DeleteUser :execresult
DELETE FROM Users WHERE id = ?;

-- name: UpdateUser :execresult
UPDATE Users SET username = ?, password_hash = ? WHERE id = ?;

-- name: CreateMessage :execresult
INSERT INTO Messages (sender_id, receiver_id, payload)
VALUES (sender_user_id_value, receiver_user_id_value, 'Your message payload');

-- name: ReadMessage :one
SELECT * FROM Messages WHERE receiver_id = receiver_user_id_value LIMIT 1;

-- name: DeleteMessage :execresult
DELETE FROM Messages WHERE message_id = read_message_id_value;

-- name: CreateFile :execresult
INSERT INTO Files (owner_id, permissions, filename)
VALUES (file_owner_user_id_value, file_permissions_value, 'file_name.txt');

-- name: CreateDirectory :execresult
INSERT INTO Directories (owner_id, parent_directory_id, directory_name)
VALUES (directory_owner_user_id_value, parent_directory_id_value, 'directory_name');

-- name: CreateBlock :execresult
INSERT INTO Blocks (owner_id, block_order, block_data)
VALUES (block_owner_user_id_value, block_order_value, 'block_data_blob');
