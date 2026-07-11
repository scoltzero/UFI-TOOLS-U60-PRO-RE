// SPDX-License-Identifier: AGPL-3.0-or-later

package ztebackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"path"
	"time"
)

const (
	InnerArchiveName = "tmp/back_parameter_r1.tgz"
	InnerDigestName  = "tmp/back_parameter_r.md5"
	OutputName       = "backup_restore.bin"
)

type File struct {
	Path string
	Mode int64
	Data []byte
}

type Builder struct {
	Password []byte
	Rand     io.Reader
	Now      func() time.Time
}

func (b Builder) Build(files []File) ([]byte, error) {
	if len(b.Password) == 0 {
		return nil, errors.New("backup password is required")
	}
	if len(files) == 0 {
		return nil, errors.New("at least one backup file is required")
	}
	now := b.Now
	if now == nil {
		now = time.Now
	}
	inner, err := createTarGzip(files, now())
	if err != nil {
		return nil, err
	}
	digest := fmt.Sprintf("%x", md5.Sum(inner))
	outer, err := createTarGzip([]File{
		{Path: InnerArchiveName, Mode: 0o600, Data: inner},
		{Path: InnerDigestName, Mode: 0o600, Data: []byte(digest)},
	}, now())
	if err != nil {
		return nil, err
	}
	random := b.Rand
	if random == nil {
		random = rand.Reader
	}
	return encryptOpenSSL3DES(outer, b.Password, random)
}

func createTarGzip(files []File, timestamp time.Time) ([]byte, error) {
	var output bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	gzipWriter.Header.ModTime = timestamp.UTC()
	gzipWriter.Header.OS = 3
	tarWriter := tar.NewWriter(gzipWriter)
	for _, file := range files {
		clean := path.Clean(file.Path)
		if clean == "." || clean != file.Path || path.IsAbs(clean) || len(clean) >= 3 && clean[:3] == "../" {
			return nil, fmt.Errorf("unsafe backup path %q", file.Path)
		}
		mode := file.Mode
		if mode == 0 {
			mode = 0o600
		}
		header := &tar.Header{
			Name: clean, Mode: mode, Size: int64(len(file.Data)), ModTime: timestamp.UTC(),
			Typeflag: tar.TypeReg, Format: tar.FormatGNU,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := tarWriter.Write(file.Data); err != nil {
			return nil, err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func encryptOpenSSL3DES(plaintext, password []byte, random io.Reader) ([]byte, error) {
	salt := make([]byte, 8)
	if _, err := io.ReadFull(random, salt); err != nil {
		return nil, err
	}
	keyMaterial := evpBytesToKey(password, salt, 32)
	block, err := des.NewTripleDESCipher(keyMaterial[:24])
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, keyMaterial[24:32]).CryptBlocks(ciphertext, padded)
	result := make([]byte, 0, 16+len(ciphertext))
	result = append(result, []byte("Salted__")...)
	result = append(result, salt...)
	result = append(result, ciphertext...)
	return result, nil
}

func decryptOpenSSL3DES(encrypted, password []byte) ([]byte, error) {
	if len(encrypted) < 24 || !bytes.Equal(encrypted[:8], []byte("Salted__")) || (len(encrypted)-16)%des.BlockSize != 0 {
		return nil, errors.New("invalid OpenSSL 3DES envelope")
	}
	keyMaterial := evpBytesToKey(password, encrypted[8:16], 32)
	block, err := des.NewTripleDESCipher(keyMaterial[:24])
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(encrypted)-16)
	cipher.NewCBCDecrypter(block, keyMaterial[24:32]).CryptBlocks(plaintext, encrypted[16:])
	return pkcs7Unpad(plaintext, block.BlockSize())
}

func evpBytesToKey(password, salt []byte, size int) []byte {
	result := make([]byte, 0, size)
	var previous []byte
	for len(result) < size {
		hash := md5.New()
		hash.Write(previous)
		hash.Write(password)
		hash.Write(salt)
		previous = hash.Sum(nil)
		result = append(result, previous...)
	}
	return result[:size]
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	result := make([]byte, len(data)+padding)
	copy(result, data)
	for index := len(data); index < len(result); index++ {
		result[index] = byte(padding)
	}
	return result
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("invalid padded data")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, errors.New("invalid padding")
	}
	for _, value := range data[len(data)-padding:] {
		if int(value) != padding {
			return nil, errors.New("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}
