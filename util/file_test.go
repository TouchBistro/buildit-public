package util

import (
	"path/filepath"
	"testing"
)

func TestListMatchingFilesNamesGlob(t *testing.T) {

	pattern := "file*.go"

	files, err := ListMatchingFilenames(pattern)
	if err != nil {
		t.Fail()
	}

	if len(files) != 2 {
		t.Errorf("expected 2, returned %v", len(files))
	}

	if files[1] != "file_test.go" {
		t.Errorf("expected file_test.go, returned %v", files[1])
	}

}

func TestListMatchingFilesNamesFile(t *testing.T) {

	pattern := "file.go"

	files, err := ListMatchingFilenames(pattern)
	if err != nil {
		t.Fail()
	}

	if len(files) != 1 {
		t.Errorf("expected 1, returned %v", len(files))
	}

	if files[0] != "file.go" {
		t.Errorf("expected file.go, returned %v", files[0])
	}

}

func TestListMatchingFilesNamesDir(t *testing.T) {

	pattern := "../cmd"

	files, err := ListMatchingFilenames(pattern)
	if err != nil {
		t.Fail()
	}

	if len(files) != 6 {
		t.Errorf("expected 6, returned %v", len(files))
	}

	if filepath.Base(files[0]) != "apply.go" {
		t.Errorf("expected apply.go, returned %v", files[0])
	}

}

func TestListMatchingFilesNamesDirExt(t *testing.T) {

	pattern := "."

	files, err := ListMatchingFilenames(pattern, ".go")
	if err != nil {
		t.Fail()
	}

	if len(files) != 10 {
		t.Errorf("expected 10, returned %v", len(files))
	}

	if filepath.Base(files[0]) != "coelesce.go" {
		t.Errorf("expected coelesce.go, returned %v", files[0])
	}

}
