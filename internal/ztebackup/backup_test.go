// SPDX-License-Identifier: AGPL-3.0-or-later

package ztebackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestBuildRoundTripStructure(t *testing.T) {
	files := []File{
		{Path: "data/etc/rc.local", Mode: 0o755, Data: []byte("#!/bin/sh\n")},
		{Path: "data/data/ufi-tools/service.sh", Mode: 0o755, Data: []byte("service")},
		{Path: "data/data/ufi-tools/ufi-tools-u60pro", Mode: 0o755, Data: []byte("binary")},
	}
	builder := Builder{
		Password: []byte("test-password"), Rand: bytes.NewReader([]byte("12345678")),
		Now: func() time.Time { return time.Unix(1, 0) },
	}
	encrypted, err := builder.Build(files)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := decryptOpenSSL3DES(encrypted, builder.Password)
	if err != nil {
		t.Fatal(err)
	}
	outerFiles := readTarGzip(t, outer)
	inner := outerFiles[InnerArchiveName]
	wantDigest := fmt.Sprintf("%x", md5.Sum(inner))
	if string(outerFiles[InnerDigestName]) != wantDigest {
		t.Fatal("inner archive MD5 mismatch")
	}
	innerFiles := readTarGzip(t, inner)
	for _, file := range files {
		if !bytes.Equal(innerFiles[file.Path], file.Data) {
			t.Fatalf("missing or changed %s", file.Path)
		}
	}
}

func TestRejectUnsafePath(t *testing.T) {
	_, err := (Builder{Password: []byte("password")}).Build([]File{{Path: "../etc/passwd", Data: []byte("x")}})
	if err == nil {
		t.Fatal("expected path rejection")
	}
}

func readTarGzip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	files := map[string][]byte{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = content
	}
	return files
}
