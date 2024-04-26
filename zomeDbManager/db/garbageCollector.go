package db

import (
	"database/sql"
	"time"
)

// DeleteOldMessagesAndFiles deletes messages older than two weeks and files past their expiry date
func DeleteOldMessagesAndFiles(db *sql.Tx, expiryDate time.Time, deleteExpiredFiles bool) error {

	// Delete old messages
	_, err := db.Exec("DELETE FROM Messages WHERE timestamp < ?", expiryDate)
	if err != nil {
		return err
	}

	if !deleteExpiredFiles {
		return nil
	}

	// Delete expired files and their blocks
	_, err = db.Exec(`
		DELETE FROM Files WHERE expiry IS NOT NULL AND expiry < CURRENT_TIMESTAMP;
		DELETE FROM Blocks WHERE part_id IN (SELECT id FROM Files WHERE expiry IS NOT NULL AND expiry < CURRENT_TIMESTAMP);
	`)
	if err != nil {
		return err
	}

	return nil
}

// garbageCollector periodically deletes old messages and expired files
func GarbageCollector(dbInst *sql.Tx, expiryDur time.Duration, deleteExpiredFiles bool) error {
	for {
		// Calculate the expiry date for messages
		expiryDate := time.Now().Add(-expiryDur)

		// Delete old messages and expired files
		err := DeleteOldMessagesAndFiles(dbInst, expiryDate, deleteExpiredFiles)
		if err != nil {
			return err
		}

		// Sleep for 1 hour before running the garbage collector again
		time.Sleep(time.Hour)
	}
}
