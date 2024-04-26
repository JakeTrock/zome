package db

import (
	"database/sql"
)

type Block struct {
	ID         int64
	PartID     int64
	BlockOrder int64
	BlockData  []byte
}

type Directory struct {
	ID                int64
	OwnerID           int64
	GroupID           sql.NullInt64
	Acl               int64
	ParentDirectoryID sql.NullInt64
	DirectoryName     string
	CreatedAt         sql.NullTime
	Expiry            sql.NullTime
}

type File struct {
	ID          int64
	OwnerID     int64
	GroupID     sql.NullInt64
	Acl         int64
	Filename    string
	DirectoryID int64
	Length      int64
	Hash        string
	CreatedAt   sql.NullTime
	Expiry      sql.NullTime
}

type Group struct {
	ID        int64
	GroupName string
	CreatedAt sql.NullTime
}

type Message struct {
	ID         int64
	SenderID   int64
	ReceiverID int64
	Payload    string
	Timestamp  sql.NullTime
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    sql.NullTime
	LastLogin    sql.NullTime
}

type UserGroup struct {
	UserID  sql.NullInt64
	GroupID sql.NullInt64
}

// TODO: is this a proper way to handle this?

// condition 1: you are the owner
// condition 2: you are in the group and the file is group readable
// condition 3: the file is world readable
// helpful: https://chmod-calculator.com/
const hasRead = `(((acl LIKE '7__' OR acl LIKE '5__' OR acl LIKE '4__' OR acl LIKE '6__') AND owner_id = ?)
	OR ((acl LIKE '_7_' OR acl LIKE '_5_' OR acl LIKE '_4_' OR acl LIKE '_6_') AND group_id = ?)
	OR (acl LIKE '__7' OR acl LIKE '__5' OR acl LIKE '__4' OR acl LIKE '__6'))`

const hasWrite = `(((acl LIKE '2__' OR acl LIKE '7__' OR acl LIKE '6__' OR acl LIKE '3__') AND group_id = ?)
	OR ((acl LIKE '_2_' OR acl LIKE '_7_' OR acl LIKE '_6_' OR acl LIKE '_3_') AND group_id = ?)
	OR (acl LIKE '__2' OR acl LIKE '__7' OR acl LIKE '__6' OR acl LIKE '__3'))`

const hasExecute = `(((acl LIKE '1__' OR acl LIKE '5__' OR acl LIKE '7__' OR acl LIKE '3__') AND group_id = ?)
	OR ((acl LIKE '_1_' OR acl LIKE '_5_' OR acl LIKE '_7_' OR acl LIKE '_3_') AND group_id = ?)
	OR (acl LIKE '__1' OR acl LIKE '__5' OR acl LIKE '__7' OR acl LIKE '__3'))`
