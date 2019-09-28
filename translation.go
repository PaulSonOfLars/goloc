package goloc

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

var data = make(map[string]map[string]Value)
var dataCount = make(map[string]int)
var languages []string
var DefaultLang = "en-GB"

func Trnl(lang string, trnlVal string) string {
	v, ok := data[lang][trnlVal]
	if !ok || v.Value == "" {
		return data[DefaultLang][trnlVal].Value
	}
	return v.Value
}

func Trnlf(lang string, trnlVal string, dataMap map[string]string) string {
	var replData []string
	for k, v := range dataMap {
		replData = append(replData, "{"+k+"}", v)
	}
	repl := strings.NewReplacer(replData...)
	v, ok := data[lang][trnlVal]
	if !ok || v.Value == "" {
		return repl.Replace(data[DefaultLang][trnlVal].Value)
	}
	return repl.Replace(v.Value)
}

func Add(text string) string {
	logrus.Warn("unloaded translation string for Add()")
	return text
}

func Addf(text string, format ...interface{}) string {
	logrus.Warn("unloaded translation string for Addf()")
	return fmt.Sprintf(text, format...)
}

func LoadAll(defLang string) {
	base := path.Join(translationDir, defLang)
	err := filepath.Walk(base,
		func(fpath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(base, fpath)
			if err != nil {
				logrus.WithError(err).Errorf("Could not get relative path of %s", fpath)
			}
			Load(relPath)
			//Load(info.Name())
			return nil
		})
	if err != nil {
		logrus.WithError(err).Errorf("Failed to walk translations directory %s", base)
	}
}

func LoadLangAll(lang string) {
	base := path.Join(translationDir, lang)
	err := filepath.Walk(base,
		func(fpath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(base, fpath)
			if err != nil {
				logrus.WithError(err).Errorf("Could not get relative path of %s", fpath)
			}
			LoadLangModule(lang, relPath)
			//Load(info.Name())
			return nil
		})
	if err != nil {
		logrus.WithError(err).Errorf("Failed to walk translations directory %s", base)
	}
}

func LoadLangModule(lang string, moduleName string) {
	f, err := os.Open(path.Join(translationDir, lang, strings.TrimSuffix(moduleName, path.Ext(moduleName))+".xml"))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		logrus.WithError(err).Errorf("Failed to open file at %s", moduleName)
		return
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	var xmlData Translation
	err = dec.Decode(&xmlData)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to decode data for %s", moduleName)
		return
	}
	for _, row := range xmlData.Rows {
		if _, ok := data[path.Base(lang)]; !ok {
			data[path.Base(lang)] = make(map[string]Value)
		}
		if row.Name == "" { // ignore empties
			continue
		}
		data[path.Base(lang)][row.Name] = row
	}
	count := xmlData.Counter
	if count <= 0 {
		count = len(xmlData.Rows)
	}
	if dataCount[moduleName] < count {
		dataCount[moduleName] = count
	}
}

func Load(moduleToLoad string) {
	files, err := ioutil.ReadDir(translationDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}

		logrus.WithError(err).Errorf("failed to load %s", moduleToLoad)
		return
	}
	for _, x := range files {
		if !x.IsDir() || strings.HasPrefix(x.Name(), ".") {
			// if not a directory, or is hidden, skip
			continue
		}

		LoadLangModule(x.Name(), moduleToLoad)
	}
}

func Languages() (ss []string) {
	if languages != nil {
		return languages
	}
	for k := range data {
		ss = append(ss, k)
	}
	sort.Strings(ss)
	languages = ss
	return ss
}

func IsLangSupported(s string) bool {
	_, ok := data[s]
	return ok
}
