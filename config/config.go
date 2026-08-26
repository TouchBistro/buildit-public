package config

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/text"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

// Parse looks at all the supplied paths for configs, overrides, templates & includes;
// convert them to a list of files & then merge them using the supplied templates, includes
// & overrides, with variables subsitutions
func Parse(ctx context.Context, opts RootOptions, tokens map[string]any) (*InternalConfig, error) {

	// collect files from all supplied paths
	var configFileNames []string
	for _, path := range opts.ConfigPath {
		f, err := util.ListMatchingFilenames(path, ".yml", ".yaml")
		if err != nil {
			return nil, err
		}
		configFileNames = append(configFileNames, f...)
	}

	var err error
	ic := &InternalConfig{}

	if len(opts.TemplatesPath) > 0 {
		log.Debugf("using templates for merging configs")
		var tmpl *template.Template
		if tmpl, err = parseTemplates(opts.TemplatesPath); err != nil {
			return nil, err
		}
		for _, fileName := range configFileNames {
			var bc *builditConfig
			if bc, err = genWithTemplates(fileName, tmpl, tokens); err != nil {
				return nil, err
			}
			ic.Collect(*bc)

		}
	} else {
		var incl []byte
		if len(opts.IncludePath) > 0 {
			log.Debugf("using include files for merging configs")
			if incl, err = parseIncludes(opts.IncludePath, ".yml", ".yaml"); err != nil {
				return nil, err
			}
		}
		for _, fileName := range configFileNames {
			var bc *builditConfig
			if bc, err = genWithIncludes(fileName, incl, tokens); err != nil {
				return nil, err
			}
			ic.Collect(*bc)
		}
	}

	// add override to the value created above
	var oc *builditConfig
	if oc, err = parseOverrides(opts.OverridePath, tokens); err != nil {
		return nil, err
	}
	ic._override = oc

	if err := ic.Generate(ctx, opts); err != nil {
		return nil, err
	}

	return ic, nil
}

// parseOverrides check if there is override config supplied, if yes then we
// read that file, parse it & prepare for a merge this into the target
// builditConfig.
func parseOverrides(paths []string, tokens map[string]any) (*builditConfig, error) {

	if len(paths) > 1 {
		return nil, errors.Errorf(
			"merging override config is not supported, supply an override path that resolves to a single file")
	}

	var err error
	var files []string
	var overrideConfig *builditConfig

	if len(paths) == 0 {
		return overrideConfig, nil
	}

	path := paths[0] // only consider the first one...
	if len(path) > 0 {
		log.Infof("Reading override configuration from %q", path)
		files, err = util.ListMatchingFilenames(path, ".yml", ".yaml")
		if err != nil {
			return nil, errors.Wrapf(err,
				"failed to fetch a list of files to read for override config from path %v", path)
		}

		if len(files) > 1 {
			return nil, errors.Errorf(
				"merging override config is not supported, supply an override path that resolves to a single file")
		}

		overrideConfig, err = genWithIncludes(files[0], nil, tokens)
		if err != nil {
			return nil, errors.Wrapf(err, "error pasing override config file at %v", files[0])
		}
	}

	return overrideConfig, nil
}

// parseIncludes checks if there is include config supplied, if yes then we
// read that file, merge & return the byte stream so it can be prepended to
// config spec before parsing
func parseIncludes(pathParams []string, ext ...string) ([]byte, error) {
	var bytes []byte
	for _, pathParam := range pathParams {
		if len(pathParam) > 0 { // TODO why is this here?

			fileNames, err := util.ListMatchingFilenames(pathParam, ext...)
			if err != nil {
				return nil, errors.Wrapf(
					err, "failed to fetch a list of files to read for includes from path %v", pathParam)
			}

			for _, fileName := range fileNames {
				log.Debugf("\tincluding content from %v", fileName)
				_bytes, err := os.ReadFile(fileName)
				if err != nil {
					if errors.Is(err, os.ErrNotExist) {
						return nil, errors.Errorf("include/template file %s does not exist", fileName)
					}
					return nil, errors.Wrapf(err, "failed to read file %s", fileName)
				}
				bytes = append(bytes, _bytes...)
			}
		}
	}
	return bytes, nil
}

// parseTemplates checks if there is template config supplied, if yes then we
// read & initialize a text/template with all the included temp
func parseTemplates(pathParams []string) (*template.Template, error) {

	var err error
	var bytes []byte
	var tmpl *template.Template

	// using parse includes to get a byte stream from all files pointed
	// by the template parameter
	bytes, err = parseIncludes(pathParams, ".tmpl")
	if err != nil {
		return nil, err
	}

	// TODO for now we still try to replace any C-macro style tags
	bytes = text.ExpandVariables(bytes, toGoActionDelims)
	if tmpl, err = initTemplate("buildit").Parse(string(bytes)); err != nil {
		return nil, err
	}

	return tmpl, nil
}

func getValue(data any, key string) (any, error) {

	if val := viper.Get(key); val != nil {
		return val, nil
	} else {
		return nil, fmt.Errorf("buildit: value for key %q is not supplied, or nil", key)
	}
}

// generate builditConfig for the supplied config @ path
func genWithTemplates(path string, tmpl *template.Template, tokens map[string]any) (*builditConfig, error) {
	log.Tracef("genWithTemplates: reading buildit spec from config file template %v", path)

	var err error
	var _bytes []byte

	// read bytes from file
	if _bytes, err = os.ReadFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.Errorf("buildit config %s does not exist", path)
		}
		return nil, errors.Wrapf(err, "failed to read file %s", path)
	}

	// calculate the SHA pre-variable expansion
	sha := fmt.Sprintf("%x", sha256.Sum256(_bytes))

	_bytes = text.ExpandVariables(_bytes, toGoActionDelims)
	if tmpl, err = tmpl.New(path).Option("missingkey=error").Parse(string(_bytes)); err != nil {
		return nil, err
	}

	var buff bytes.Buffer
	if err = tmpl.ExecuteTemplate(&buff, path, tokens); err != nil {
		return nil, err
	}

	_bytes = buff.Bytes()
	printConfigFile(path, _bytes) // trace

	config := &builditConfig{}
	err = yaml.Unmarshal(_bytes, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse yaml file at %s", path)
	}
	config.SHA = sha
	config.Source = path
	return config, nil
}

// generate builditConfig for the supplied config @ path & include templates as []byte
func genWithIncludes(path string, includes []byte, tokens map[string]any) (*builditConfig, error) {
	log.Tracef("genWithIncludes reading buildit spec from config file template %v", path)

	var err error
	var _bytes []byte

	// read bytes from file
	if _bytes, err = os.ReadFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.Errorf("buildit config %s does not exist", path)
		}
		return nil, errors.Wrapf(err, "failed to read file %s", path)
	}

	// prepend includes content is supplied
	if len(includes) > 0 {
		_bytes = append(includes, _bytes...)
	}

	// calculate the SHA pre-variable expansion
	sha := fmt.Sprintf("%x", sha256.Sum256(_bytes))

	// converting any legacy buildit's c-macro style to go tmpl actions {{.var}}
	_bytes = text.ExpandVariables(_bytes, toGoActionDelims)

	// var err error
	var tmpl *template.Template
	// if tmpl, err = template.New(path).Option("missingkey=error").Parse(string(_bytes)); err != nil {
	if tmpl, err = initTemplate(path).Parse(string(_bytes)); err != nil {
		return nil, err
	}

	var buff bytes.Buffer
	// ExecuteTemplate is used to ensure only tmpl intended here is executed
	// This would dis-regard any {{ define name }} tags included inside the text
	if err = tmpl.ExecuteTemplate(&buff, path, tokens); err != nil {
		return nil, err
	}

	_bytes = buff.Bytes()
	printConfigFile(path, _bytes) // trace

	config := &builditConfig{}
	err = yaml.Unmarshal(_bytes, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse yaml file at %s", path)
	}
	config.SHA = sha
	config.Source = path
	return config, nil
}

// toGoTextActionMacroDelimiter converts ${Var} style builit variable
// to go text/template actions {{ .var }}
func toGoActionDelims(in string) string {
	return fmt.Sprintf(`{{getValue . "%v"}}`, strings.ToLower(in))
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"getValue": getValue,
	}
}

func initTemplate(name string) *template.Template {
	return template.New(name).Option("missingkey=error").Funcs(templateFuncs())
}

func printConfigFile(path string, _bytes []byte) {

	scanner := bufio.NewScanner(bytes.NewReader(_bytes))

	log.Tracef("-----------------------")
	log.Tracef("%v", path)
	log.Tracef("-----------------------")
	lno := 1
	for scanner.Scan() {
		log.Tracef("%7d: %v", lno, scanner.Text())
		lno++
	}
	log.Tracef("-----------------------")

}
