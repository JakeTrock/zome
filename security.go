package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

func NewAESKey() ([]byte, error) {
	// Generate a new AES key
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func AesGCMEncrypt(key, data []byte) ([]byte, error) {
	// Encrypt the data with the key
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return ciphertext, nil
}

func AesGCMDecrypt(key, data []byte) ([]byte, error) {
	// Decrypt the data with the key
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func sanitizeACL(aclString string) (string, error) {
	// acl is a 2 character int determining what other sites can access the data
	// posn 1 = group
	// posn 2 = other
	// 11 = r- r-
	// 22 = -w -w
	// 33 = rw rw
	println(aclString)

	keyRegex := "^[1-3]{2}$"

	println(regexp.MustCompile(keyRegex).MatchString(aclString))

	if !regexp.MustCompile(keyRegex).MatchString(aclString) {
		return "", fmt.Errorf("invalid acl")
	}
	// convert the acl to a string
	return aclString, nil
}

func checkACL(acl, operation string, domainSource, domainTarget string) bool {
	// operation is a number between 1 and 7
	opRegex := "^[1-3]{1}$"
	if !regexp.MustCompile(opRegex).MatchString(operation) {
		return false
	}
	// check if the domainSource has the right to perform the operation on domainTarget
	// if the domainSource is the same as the domainTarget, the operation is always allowed
	if domainSource == domainTarget {
		return true
	} else {
		aclPos := 1
		if domainSource[:len(domainSource)-1] == domainTarget[:len(domainTarget)-1] {
			aclPos = 0
		}
		// if the domainSource is in the same group as the domainTarget
		operationNum, err := strconv.Atoi(operation)
		if err != nil {
			logger.Error(err)
			return false
		}
		return operationNum >= int(acl[aclPos])
	}
}
