package goloc

import (
	"encoding/xml"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

var data = make(map[string]map[string]Value)

// todo: get some defaul values going
func Trnl(lang string, trnlVal string) string {
	return data[lang][trnlVal].Value
}

func Trnlf(lang string, trnlVal string, dataMap map[string]string) string {
	var replData []string
	for k, v := range dataMap {
		replData = append(replData, "{"+k+"}", v)
	}
	repl := strings.NewReplacer(replData...)
	return repl.Replace(data[lang][trnlVal].Value)
}

func Load(moduleToLoad string) {
	files, err := ioutil.ReadDir(translationDir)
	if err != nil {
		logrus.Fatal(err)
		return
	}
	for _, x := range files {
		func() {
			f, err := os.Open(path.Join(translationDir, x.Name(), strings.TrimSuffix(moduleToLoad, path.Ext(moduleToLoad))+".xml"))
			if err != nil {
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
		}()
	}
}
