package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckACL(t *testing.T) {
	// Test case 1: domainSource is the same as domainTarget
	if !checkACL("33", "1", "example.com", "example.com") {
		t.Error("Expected checkACL to return true, but got false")
	}

	// Test case 2: domainSource is in the same group as domainTarget and operation is allowed
	if !checkACL("22", "2", "example.com", "sub.example.com") {
		t.Error("Expected checkACL to return true, but got false")
	}

	// Test case 3: domainSource is in the same group as domainTarget but operation is not allowed
	if checkACL("11", "3", "example.com", "sub.example.com") {
		t.Error("Expected checkACL to return false, but got true")
	}

	// Test case 4: domainSource is not in the same group as domainTarget
	if !checkACL("23", "1", "example.com", "another.com") {
		t.Error("Expected checkACL to return true, but got false")
	}

	// Test case 5: invalid operation
	if checkACL("22", "4", "example.com", "example.com") {
		t.Error("Expected checkACL to return false, but got true")
	}
}

func TestAesGCMDecrypt(t *testing.T) {
	key := []byte("0123456789abcdef")
	expectedPlaintext := []byte("Hello,{}222,\"\"World!")

	// Test case 1: Decrypting valid data
	cipherText, err := AesGCMEncrypt(key, expectedPlaintext)
	assert.NoError(t, err)

	plaintext, err := AesGCMDecrypt(key, cipherText)
	assert.NoError(t, err)
	if !bytes.Equal(plaintext, expectedPlaintext) {
		t.Errorf("Expected plaintext to be %v, but got %v", expectedPlaintext, plaintext)
	}

	// Test case 2: Decrypting invalid data
	invalidData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e}
	_, err = AesGCMDecrypt(key, invalidData)
	assert.Error(t, err)
}

func TestFileSha(t *testing.T) {
	// Test case 1: Valid file
	fileSha, err := sha256File("./testPath/testfile.jpg")
	assert.NoError(t, err)
	assert.Equal(t, "8e9213d773364a27a52a32c5db1bbef16b83bddcc0b9d070f6f345795fef2aaf", fileSha)
}
