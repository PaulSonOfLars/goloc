package goloc

import (
	"encoding/xml"
	"go/ast"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

func parseFmtString(rdata []rune, ret *ast.CallExpr) (newData []rune, mapData []ast.Expr, needStrconv bool) {
	index := 1
	for i := 0; i < len(rdata); i++ {
		if rdata[i] == '%' && i+1 < len(rdata) {
			i++
			switch x := rdata[i]; x {
			case 's': // string -> no change
				mapData = append(mapData,
					&ast.KeyValueExpr{
						Key: &ast.BasicLit{
							Kind:  token.STRING,
							Value: strconv.Quote(strconv.Itoa(index)),
						},
						Value: ret.Args[index],
					})
			case 'd': // int
				mapData = append(mapData,
					&ast.KeyValueExpr{
						Key: &ast.BasicLit{
							Kind:  token.STRING,
							Value: strconv.Quote(strconv.Itoa(index)),
						},
						Value: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   &ast.Ident{Name: "strconv"},
								Sel: &ast.Ident{Name: "Itoa"},
							},
							Args: []ast.Expr{ret.Args[index]},
						},
					})
				needStrconv = true
			case 't': // bool
				mapData = append(mapData,
					&ast.KeyValueExpr{
						Key: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"` + strconv.Itoa(index) + `"`,
						},
						Value: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   &ast.Ident{Name: "strconv"},
								Sel: &ast.Ident{Name: "FormatBool"},
							},
							Args: []ast.Expr{ret.Args[index]},
						},
					})
				// case 'p': // pointer (wtaf)
				// strconv
			default:
				Logger.Fatalf("no way to handle '%s' formatting yet", string(x))
			}
			newData = append(newData, []rune("{"+strconv.Itoa(index)+"}")...)
			index++
		} else {
			newData = append(newData, rdata[i])
		}
	}
	return newData, mapData, needStrconv
}

func initHasLoad(ret *ast.FuncDecl, modName string) bool {
	for _, x := range ret.Body.List {
		if exp, ok := x.(*ast.ExprStmt); ok {
			if cexp, ok := exp.X.(*ast.CallExpr); ok {
				val, ok2 := cexp.Args[0].(*ast.BasicLit)
				if sexp, ok := cexp.Fun.(*ast.SelectorExpr); ok && ok2 && val.Value == strconv.Quote(modName) {
					obj, ok1 := sexp.X.(*ast.Ident)
					if ok1 && obj.Name == "goloc" && sexp.Sel.Name == "Load" {
						return true
					}
				}
			}
		}
	}
	return false
}

func sep(s string) string {
	return string(filepath.Separator) + s + string(filepath.Separator)
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if s == x {
			return true
		}
	}
	return false
}

func (l *Locer) injectTran(name string, ret *ast.CallExpr, f *ast.SelectorExpr, v *ast.BasicLit) (*ast.CallExpr, bool) {
	data, err := strconv.Unquote(v.Value)
	if err != nil {
		Logger.Fatal(err)
		return nil, false
	}
	needStrConvImport := false

	itemName, isDup := noDupStrings[data]
	if !isDup {
		dataCount[name]++
		itemName = name + ":" + strconv.Itoa(dataCount[name])
		noDupStrings[data] = itemName
		newDataNames[name] = append(newDataNames[name], itemName)
	}

	args := []ast.Expr{
		&ast.Ident{Name: "lang"},
		&ast.BasicLit{
			Kind:  token.STRING,
			Value: strconv.Quote(itemName),
		},
	}

	methToCall := "Trnl"
	if contains(l.Fmtfuncs, f.Sel.Name) || f.Sel.Name == "Addf" { // is a format call
		methToCall = "Trnlf"
		dataNew, mapData, needStrconv := parseFmtString([]rune(data), ret)
		needStrConvImport = needStrconv

		data = string(dataNew)
		args = append(args, &ast.CompositeLit{
			Type: &ast.MapType{
				Key: &ast.BasicLit{
					Kind:  token.STRING,
					Value: "string",
				},
				Value: &ast.BasicLit{
					Kind:  token.STRING,
					Value: "string",
				},
			},
			Elts: mapData,
		})
	}

	if !isDup {
		for lang := range newData {
			newData[lang][name][itemName] = Value{
				Id:      dataCount[name],
				Name:    itemName,
				Value:   "",
				Comment: data,
			}
		}
		// set data only for default value
		newData[l.DefaultLang.String()][name][itemName] = Value{
			Id:      dataCount[name],
			Name:    itemName,
			Value:   data,
			Comment: itemName,
		}
	}

	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "goloc"},
			Sel: &ast.Ident{Name: methToCall},
		},
		Args: args,
	}, needStrConvImport
}

func stringSlicesEqual(a, b []string) bool {
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// todo: simplify the newData structure
func (l *Locer) saveMap(newData map[string]map[string]map[string]Value, newDataNames map[string][]string) error {
	for lang, filenameMap := range newData {
		for modName, modData := range filenameMap {
			names := loadOriginalModuleOrder(modName)
			newNames := newDataNames[modName]
			if len(names) < len(newNames) || !stringSlicesEqual(names[len(names)-len(newNames):], newNames) {
				names = append(names, newNames...)
			}
			if len(names) == 0 {
				continue
			}

			var xmlOutput Translation
			for _, k := range names {
				langData, ok := modData[k]
				if !ok {
					langData = Value{
						Id:      -1,
						Name:    "",
						Value:   "",
						Comment: "Go ahead and ignore this; do not translate. This is an outdated string that needs to be left here because crowdin is derpy.",
					}
				}

				xmlOutput.Rows = append(xmlOutput.Rows, langData)
			}
			xmlOutput.Counter = dataCount[modName]

			err := func() error {
				// TODO: other filetypes than xml
				w := os.Stdout
				if l.Apply {
					// TODO: choose translationDir
					xmlName := strings.TrimSuffix(path.Join(translationDir, lang, modName), path.Ext(modName)) + ".xml"
					err := os.MkdirAll(filepath.Dir(xmlName), 0755)
					if err != nil {
						return err
					}
					f, err := os.Create(xmlName)
					if err != nil {
						return err
					}
					defer f.Close()
					// set encoding output
					w = f
				}
				w.WriteString(xml.Header)
				enc := xml.NewEncoder(w)
				enc.Indent("", "    ")
				if err := enc.Encode(xmlOutput); err != nil {
					return err
				}
				w.WriteString("\n")
				return nil
			}()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func loadOriginalModuleOrder(modName string) (out []string) {
	f, err := os.Open(path.Join(translationDir, DefaultLang, strings.TrimSuffix(modName, path.Ext(modName))+".xml"))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		Logger.Fatal(err)
		return
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	var xmlData Translation
	err = dec.Decode(&xmlData)
	if err != nil {
		Logger.Fatal(err)
	}
	for _, row := range xmlData.Rows {
		out = append(out, row.Name)
	}
	return out
}
