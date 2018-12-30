package goloc

import (
	"encoding/xml"
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"
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
	return fmt.Sprintf(text, format)
}

func Load(moduleToLoad string) {
	files, err := ioutil.ReadDir(translationDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}

		logrus.Fatal(err)
		return
	}
	for _, x := range files {
		func() {
			f, err := os.Open(path.Join(translationDir, x.Name(), strings.TrimSuffix(moduleToLoad, path.Ext(moduleToLoad))+".xml"))
			if err != nil {
				if os.IsNotExist(err) {
					return
				}
				logrus.Fatal(err)
				return
			}
			defer f.Close()
			dec := xml.NewDecoder(f)
			var xmlData Translation
			err = dec.Decode(&xmlData)
			if err != nil {
				logrus.Fatal(err)
				return
			}
			for _, row := range xmlData.Rows {
				if _, ok := data[path.Base(x.Name())]; !ok {
					data[path.Base(x.Name())] = make(map[string]Value)
				}
				data[path.Base(x.Name())][row.Name] = row
			}
			count := xmlData.Counter
			if count <= 0 {
				count = len(xmlData.Rows)
			}
			if dataCount[moduleToLoad] < count {
				dataCount[moduleToLoad] = count
			}
		}()
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
