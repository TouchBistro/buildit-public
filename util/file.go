package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// ListMatchingFilenames returns a file, or files in scope given the supplied path
//
// if the path represents a glob pattern, then all matching files are returned, else if
// the path exists as a directory, & optionally a list of extensions are supplied, then
// all files in the directory, with the matching extension are returned. If no extensions
// are supplied, then all files are returned.
// if the path points to a file, then the it is returned
// if no match, an empty []string, a nil error is return
// Any errors during parsing the glob pattern, listing or checking files returns a non-nil
// error
func ListMatchingFilenames(path string, extensions ...string) ([]string, error) {

	log.Debugf("listing matching files names for %v, extensions: %v", path, extensions)

	var err error
	var names []string
	globChars := "?*[{}]"

	// is the supplied path a glob?
	if strings.ContainsAny(path, globChars) {
		log.Tracef("treating path value %v as a glob", path)
		names, err = filepath.Glob(path)
		if err != nil {
			return nil, err
		}
	} else {

		// now let's check if this is a valid filesystem path
		info, err := os.Stat(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error checking file stat %q", path)
		}

		// is a directory
		if info.IsDir() {
			log.Tracef("treating path value %v as a directory", path)
			// prepare extension set to quickly filter out files in this directory
			exts := make(map[string]struct{})
			for _, e := range extensions {
				if !strings.HasPrefix(e, ".") {
					e = fmt.Sprintf(".%s", e)
				}
				exts[e] = struct{}{}
			}

			entries, err := os.ReadDir(path)
			if err != nil {
				return nil, errors.Wrapf(err, "error listing contents of dir %q", path)
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					// include files with supplied extensions
					ext1 := filepath.Ext(entry.Name())
					if _, ok := exts[ext1]; ok || len(exts) == 0 {
						names = append(names, filepath.Join(path, entry.Name()))
					}
				}
			}
		} else { // is a single file
			log.Tracef("treating path value %v as a filename", path)
			names = []string{path}
		}
	}

	PrintFileNames(names)
	log.Debugf("%v files found", len(names))
	return names, nil

}

func PrintFileNames(names []string) {
	log.Tracef("returning %v files in scope", len(names))
	for n, f := range names {
		log.Tracef("%v. filename: %v", n, f)
	}
}
