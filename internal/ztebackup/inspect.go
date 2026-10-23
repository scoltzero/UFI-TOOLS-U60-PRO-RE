// SPDX-License-Identifier: AGPL-3.0-or-later

package ztebackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

type Report struct {
	EncryptedBytes int               `json:"encrypted_bytes"`
	InnerBytes     int               `json:"inner_bytes"`
	InnerMD5       string            `json:"inner_md5"`
	Paths          []string          `json:"paths"`
	Symlinks       map[string]string `json:"symlinks,omitempty"`
}

type archiveEntry struct {
	data     []byte
	mode     int64
	typeflag byte
	linkname string
}

func Inspect(encrypted, password []byte) (Report, error) {
	report := Report{EncryptedBytes: len(encrypted)}
	outer, err := decryptOpenSSL3DES(encrypted, password)
	if err != nil {
		return report, fmt.Errorf("decrypt backup: %w", err)
	}
	outerFiles, err := inspectTarGzip(outer, 128<<20)
	if err != nil {
		return report, fmt.Errorf("read outer archive: %w", err)
	}
	innerEntry, okInner := outerFiles[InnerArchiveName]
	digestEntry, okDigest := outerFiles[InnerDigestName]
	if !okInner || !okDigest || len(outerFiles) != 2 {
		return report, errors.New("outer archive does not contain the exact U60 backup pair")
	}
	if !innerEntry.regular() || !digestEntry.regular() {
		return report, errors.New("outer U60 backup pair must contain regular files")
	}
	inner, digest := innerEntry.data, digestEntry.data
	actual := fmt.Sprintf("%x", md5.Sum(inner))
	if !strings.EqualFold(strings.TrimSpace(string(digest)), actual) {
		return report, errors.New("inner archive MD5 mismatch")
	}
	innerFiles, err := inspectTarGzip(inner, 512<<20)
	if err != nil {
		return report, fmt.Errorf("read inner archive: %w", err)
	}
	report.InnerBytes, report.InnerMD5 = len(inner), actual
	for name, entry := range innerFiles {
		report.Paths = append(report.Paths, name)
		if entry.typeflag == tar.TypeSymlink {
			if report.Symlinks == nil {
				report.Symlinks = make(map[string]string)
			}
			report.Symlinks[name] = entry.linkname
		}
	}
	sort.Strings(report.Paths)
	return report, nil
}

func (e archiveEntry) regular() bool {
	return e.typeflag == tar.TypeReg || e.typeflag == tar.TypeRegA
}

func inspectTarGzip(data []byte, max int64) (map[string]archiveEntry, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	files := map[string]archiveEntry{}
	var total int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		clean := path.Clean(header.Name)
		if !safeArchivePath(clean) || clean != header.Name {
			return nil, fmt.Errorf("unsafe archive entry %q", header.Name)
		}
		if _, exists := files[clean]; exists || header.Size < 0 || total+header.Size > max {
			return nil, fmt.Errorf("duplicate or oversized archive entry %q", clean)
		}
		entry := archiveEntry{typeflag: header.Typeflag, mode: header.Mode}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			body, err := io.ReadAll(io.LimitReader(tarReader, header.Size+1))
			if err != nil || int64(len(body)) != header.Size {
				return nil, fmt.Errorf("read archive entry %q", clean)
			}
			entry.data = body
		case tar.TypeSymlink:
			if header.Size != 0 || !safeSymlinkTarget(clean, header.Linkname) {
				return nil, fmt.Errorf("unsafe archive symlink %q -> %q", clean, header.Linkname)
			}
			entry.linkname = header.Linkname
		default:
			return nil, fmt.Errorf("unsupported archive entry type %q for %q", header.Typeflag, clean)
		}
		total += header.Size
		files[clean] = entry
	}
	return files, nil
}

// ReadFile returns only an explicitly selected regular file from a verified
// backup. It never extracts paths to disk or exposes unrelated configuration.
func ReadFile(encrypted, password []byte, name string) (File, error) {
	if _, err := Inspect(encrypted, password); err != nil {
		return File{}, err
	}
	outer, err := decryptOpenSSL3DES(encrypted, password)
	if err != nil {
		return File{}, err
	}
	entries, err := inspectTarGzip(outer, 128<<20)
	if err != nil {
		return File{}, err
	}
	inner, err := inspectTarGzip(entries[InnerArchiveName].data, 512<<20)
	if err != nil {
		return File{}, err
	}
	entry, ok := inner[name]
	if !ok || !entry.regular() {
		return File{}, fmt.Errorf("required regular backup file missing: %s", name)
	}
	return File{Path: name, Mode: entry.mode, Data: append([]byte(nil), entry.data...)}, nil
}

func safeArchivePath(name string) bool {
	return name != "." && !path.IsAbs(name) && name != ".." && !strings.HasPrefix(name, "../")
}

func safeSymlinkTarget(name, target string) bool {
	if target == "" || path.IsAbs(target) {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	return safeArchivePath(resolved)
}
