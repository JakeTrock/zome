package zcrypto

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckACL(t *testing.T) {
	// Test case 1: domainSource is the same as domainTarget
	if !CheckACL("33", "1", "example.com", "example.com") {
		t.Error("Expected CheckACL to return true, but got false")
	}

	// Test case 2: domainSource is in the same group as domainTarget and operation is allowed
	if !CheckACL("22", "2", "example.com", "sub.example.com") {
		t.Error("Expected CheckACL to return true, but got false")
	}

	// Test case 3: domainSource is in the same group as domainTarget but operation is not allowed
	if CheckACL("11", "3", "example.com", "sub.example.com") {
		t.Error("Expected CheckACL to return false, but got true")
	}

	// Test case 4: domainSource is not in the same group as domainTarget
	if !CheckACL("23", "1", "example.com", "another.com") {
		t.Error("Expected CheckACL to return true, but got false")
	}

	// Test case 5: invalid operation
	if CheckACL("22", "4", "example.com", "example.com") {
		t.Error("Expected CheckACL to return false, but got true")
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
	fileSha, err := Sha256File("../testPath/testfile.jpg")
	assert.NoError(t, err)
	assert.Equal(t, "8e9213d773364a27a52a32c5db1bbef16b83bddcc0b9d070f6f345795fef2aaf", fileSha)
}
func TestGenerateRandomKey(t *testing.T) {
	randomKey := GenerateRandomKey()

	// Test case 1: Check if the generated key has the correct length
	if len(randomKey) != 5 {
		t.Errorf("Expected generated key length to be 5, but got %d", len(randomKey))
	}

	// Test case 2: Check if the generated key contains only valid characters
	const validLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for _, char := range randomKey {
		if !strings.ContainsRune(validLetters, char) {
			t.Errorf("Generated key contains invalid character: %c", char)
		}
	}
}

func TestSanitizeACL(t *testing.T) {
	// Test case 1: Valid ACL
	aclString := "33"
	expectedResult := "33"
	result, err := SanitizeACL(aclString)
	assert.NoError(t, err)
	assert.Equal(t, expectedResult, result)

	// Test case 2: Invalid ACL
	aclString = "44"
	expectedError := "invalid acl"
	_, err = SanitizeACL(aclString)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err.Error())

	// Test case 3: Invalid ACL with special characters
	aclString = "1@"
	_, err = SanitizeACL(aclString)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err.Error())
}
