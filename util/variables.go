package util

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/TouchBistro/goutils/color"
	"github.com/TouchBistro/goutils/fatal"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// LoadVariables builds the viper singleton & sets all the key values
// from the buildit variables sources, files (from paths), environment
// and key values from --variable flags supplied as key=value per string
func LoadVariables(paths []string, env bool, kvs ...string) (map[string]any, error) {

	// load the list of files
	var err error
	var varFiles []string

	for _, path := range paths {
		var files []string
		if files, err = ListMatchingFilenames(path, ".vars"); err != nil {
			return nil, err
		}
		varFiles = append(varFiles, files...) // collect all here...
	}

	for _, varFile := range varFiles {
		v := viper.New()
		dir := filepath.Dir(varFile)
		file := filepath.Base(varFile)
		log.Tracef("loading variables from %v", varFile)

		v.AddConfigPath(dir)
		v.SetConfigName(file)
		v.SetConfigType("yaml")

		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}

		var vars any
		var ok bool
		if vars, ok = v.AllSettings()["variables"]; !ok {
			return nil, fmt.Errorf("no variables section found in file supplied - variables files > %v", varFile)
		}
		var mp map[string]any
		if mp, ok = vars.(map[string]any); !ok {
			return nil, fmt.Errorf("variables section must have a nested map! type of map[string]any - variables files > %v", varFile)
		}

		if err := viper.MergeConfigMap(mp); err != nil {
			return nil, err
		}
	}

	// merge from env, if included
	if env {
		prefix := "buildit"
		envars := os.Environ()
		if envmap, err := fillVarMapFromSlice(envars, prefix); err != nil {
			return nil, err
		} else {
			if err := viper.MergeConfigMap(envmap); err != nil {
				return nil, err
			}
		}
	}

	// merge from kvs supplied from command-line
	if kvMap, err := fillVarMapFromSlice(kvs, ""); err != nil {
		return nil, err
	} else {
		if err := viper.MergeConfigMap(kvMap); err != nil {
			return nil, err
		}
	}

	if log.IsLevelEnabled(log.DebugLevel) {
		for _, k := range viper.AllKeys() {
			log.Debugf("%v= %v", color.Magenta(k), viper.GetString(k))
		}
	}

	return viper.AllSettings(), nil
}

// fillVarMapFromSlice parses variable as key=value from each
// element in the supplied slice, if prefix is supplied, then
// a case-insensitive match is also checked before including them
// into t
func fillVarMapFromSlice(vars []string, prefix string) (map[string]any, error) {
	// read from variables; overrides any found in files in var files in scope
	dat := make(map[string]any)
	regex := regexp.MustCompile(`(.+)=(.*)`)
	for _, vr := range vars {
		matches := regex.FindStringSubmatch(vr)
		if matches == nil {
			return nil, &fatal.Error{
				Msg: fmt.Sprintf("variable must have format 'name=value', got '%s'", vr),
			}
		}
		if strings.HasPrefix(strings.ToLower(matches[1]), strings.ToLower(prefix)) {
			dat[matches[1]] = matches[2]
		}
	}
	return dat, nil
}
