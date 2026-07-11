// SPDX-License-Identifier: AGPL-3.0-or-later

package ztebackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestSafeSymlinkTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, target string
		want         bool
	}{
		{"etc/odhcp6c.user.d/hook", "../../sbin/hook", true},
		{"etc/hook", "../sbin/hook", true},
		{"etc/hook", "../../outside", false},
		{"etc/hook", "/etc/shadow", false},
		{"etc/hook", "", false},
	}
	for _, test := range tests {
		if got := safeSymlinkTarget(test.name, test.target); got != test.want {
			t.Fatalf("safeSymlinkTarget(%q, %q) = %v, want %v", test.name, test.target, got, test.want)
		}
	}
}

func TestInspectTarGzipAllowsContainedSymlink(t *testing.T) {
	t.Parallel()
	archive := makeTestArchive(t, &tar.Header{
		Name: "etc/hooks/event", Typeflag: tar.TypeSymlink, Linkname: "../../sbin/event", Mode: 0o777,
	})
	entries, err := inspectTarGzip(archive, 1024)
	if err != nil {
		t.Fatal(err)
	}
	entry := entries["etc/hooks/event"]
	if entry.typeflag != tar.TypeSymlink || entry.linkname != "../../sbin/event" {
		t.Fatalf("unexpected entry: %#v", entry)
	}
}

func TestInspectTarGzipRejectsEscapingSymlink(t *testing.T) {
	t.Parallel()
	archive := makeTestArchive(t, &tar.Header{
		Name: "etc/event", Typeflag: tar.TypeSymlink, Linkname: "../../outside", Mode: 0o777,
	})
	if _, err := inspectTarGzip(archive, 1024); err == nil {
		t.Fatal("expected escaping symlink to be rejected")
	}
}

func makeTestArchive(t *testing.T, headers ...*tar.Header) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, header := range headers {
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
