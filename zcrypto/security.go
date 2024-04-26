package zcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	mrand "math/rand"
	"os"
	"path/filepath"
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

func SanitizeACL(aclString string) (string, error) {
	// acl is a 2 character int determining what other sites can access the data
	// posn 1 = group
	// posn 2 = other
	// 11 = r- r-
	// 22 = -w -w
	// 33 = rw rw

	keyRegex := "^[1-3]{2}$"

	if !regexp.MustCompile(keyRegex).MatchString(aclString) {
		return "", fmt.Errorf("invalid acl")
	}
	// convert the acl to a string
	return aclString, nil
}

func CheckACL(acl, operation, domainSource, domainTarget string) bool {
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
			fmt.Println(err)
			return false
		}

		acstr := acl[aclPos : aclPos+1]
		aclNum, err := strconv.Atoi(acstr)
		if err != nil {
			fmt.Println(err)
			return false
		}
		return operationNum <= aclNum
	}
}

func Sha256File(writePath string) (string, error) {
	file, err := os.Open(writePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func CalculateDecryptedFileSize(filePath string) (int64, error) { //TODO: may not be correct,you should just store this in metadata
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}
	fileSize := fileInfo.Size()

	// Calculate the number of blocks in the file
	numBlocks := int(fileSize / aes.BlockSize)
	if fileSize%aes.BlockSize != 0 {
		numBlocks++
	}

	// Calculate the size of the decrypted file
	decryptedSize := int64(numBlocks * aes.BlockSize)

	// Subtract the padding size from the decrypted size
	paddingSize := int(fileSize % aes.BlockSize)
	if paddingSize > 0 {
		decryptedSize -= int64(aes.BlockSize - paddingSize)
	}

	return decryptedSize, nil
}

func GenerateRandomKey() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 5)
	for i := range b {
		b[i] = letters[mrand.Intn(len(letters))]
	}
	return string(b)
}
