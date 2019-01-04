package goloc

import (
	"bytes"
	"encoding/xml"
	"github.com/sirupsen/logrus"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"golang.org/x/text/language"
	"golang.org/x/tools/go/ast/astutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const translationDir = "trans"

type Translation struct {
	XMLName xml.Name `xml:"translation"`
	Rows    []Value
	Counter int
}

type Value struct {
	Id      int    `xml:"id,attr"`
	Name    string `xml:"name,attr"`
	Value   string `xml:"value"`
	Comment string `xml:",comment"`
}

type Locer struct {
	DefaultLang language.Tag
	Funcs       []string
	Fmtfuncs    []string
	Checked     map[string]struct{}
	OrderedVals []string
	Fset        *token.FileSet
	Apply       bool
}

func (l *Locer) Handle(args []string, hdnl func(*ast.File)) error {
	if len(args) == 0 {
		logrus.Errorln("No input provided.")
		return nil
	}
	for _, arg := range args {
		fi, err := os.Stat(arg)
		if err != nil {
			return err
		}

		switch mode := fi.Mode(); {
		case mode.IsDir():
			// do directory stuff
			logrus.Debugln("directory input")
			nodes, err := parser.ParseDir(l.Fset, arg, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			for _, n := range nodes {
				for _, f := range n.Files {
					name := l.Fset.File(f.Pos()).Name()
					if _, ok := l.Checked[name]; ok {
						continue // todo: check for file name clashes in diff packages?
					}
					l.Checked[name] = struct{}{}
					hdnl(f)
				}
			}
		case mode.IsRegular():
			// do file stuff
			logrus.Debugln("file input")
			node, err := parser.ParseFile(l.Fset, arg, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			name := l.Fset.File(node.Pos()).Name()
			if _, ok := l.Checked[name]; ok {
				continue // todo: check for file name clashes in diff packages?
			}
			l.Checked[name] = struct{}{}
			hdnl(node)
		}
	}
	logrus.Info("the following have been checked:")
	for k := range l.Checked {
		logrus.Info("  " + k)
	}
	return nil
}

// TODO: remove dup code with the fix() method
func (l *Locer) Inspect(node *ast.File) {
	var counter int
	var inMeth *ast.FuncDecl
	ast.Inspect(node, func(n ast.Node) bool {
		if ret, ok := n.(*ast.FuncDecl); ok {
			inMeth = ret

		} else if ret, ok := n.(*ast.CallExpr); ok {
			logrus.Debug("\n found a call ")
			//printer.Fprint(os.Stdout, fset, ret)
			if f, ok := ret.Fun.(*ast.SelectorExpr); ok {
				logrus.Debug("\n  found call named " + f.Sel.Name)
				if contains(append(l.Funcs, l.Fmtfuncs...), f.Sel.Name) && len(ret.Args) > 0 {
					ex := ret.Args[0]

					if v, ok := ex.(*ast.BasicLit); ok && v.Kind == token.STRING {
						buf := bytes.NewBuffer([]byte{})
						printer.Fprint(buf, l.Fset, v)
						logrus.Debugf("\n   found a string:\n%s", buf.String())

						counter++
						name := l.Fset.File(v.Pos()).Name() + ":" + strconv.Itoa(counter)
						l.OrderedVals = append(l.OrderedVals, name)

					} else if v2, ok := ex.(*ast.BinaryExpr); ok && v2.Op == token.ADD {
						// note: plz reformat not to use adds
						logrus.Debug("\n   found a binary expr instead of str; fix your code")
						//v, ok := v2.X.(*ast.BasicLit)
						//v, ok := v2.Y.(*ast.BasicLit)

					} else {
						logrus.Debugf("\n   found something else: %T", ex)

					}
				}
			}
		}
		return true
	})
	logrus.Debugln()
}

// todo: ensure import works as expected
func (l *Locer) Fix(node *ast.File) {
	name := l.Fset.File(node.Pos()).Name()
	cntnt := &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X: &ast.Ident{
					Name: "goloc",
				},
				Sel: &ast.Ident{
					Name: "Load",
				},
			},
			Args: []ast.Expr{
				&ast.BasicLit{
					Kind:  token.STRING,
					Value: strconv.Quote(name),
				},
			},
		},
	}

	// todo: investigate unnecessary "lang := " loads

	Load(name) // load current values
	logrus.Debug("module count at", dataCount[name])
	newData := make(map[string]map[string]map[string]Value) // locale:(filename:(trigger:Value))
	dataNames := make(map[string][]string)                  // filename:[]triggers
	noDupStrings := make(map[string]string)                 // map of currently loaded strings, to avoid duplicates and reduce translation efforts

	// make sure default language is loaded
	newData[l.DefaultLang.String()] = make(map[string]map[string]Value)
	newData[l.DefaultLang.String()][name] = make(map[string]Value)
	// initialise set for all other languages
	for k := range data { // initialise all languages
		newData[k] = make(map[string]map[string]Value)
		newData[k][name] = make(map[string]Value)
	}

	var needsSetting bool      // method needs the lang := arg
	var needsImporting bool    // goloc needs importing
	var needStrconvImport bool // need to import strconv
	var initExists bool        // does init method exist

	// should return to node?
	astutil.Apply(node,
		/*pre*/ func(cursor *astutil.Cursor) bool {
			n := cursor.Node()
			// get info on init call.
			if ret, ok := n.(*ast.FuncDecl); ok {
				if ret.Name.Name == "init" {
					initExists = true
				}

				// Check method calls
			} else if ret, ok := n.(*ast.CallExpr); ok {
				// determine if method is one of the validated ones
				if f, ok := ret.Fun.(*ast.SelectorExpr); ok {
					logrus.Debug("\n  found random call named " + f.Sel.Name)

					// if valid and has args, check first arg (which should be a string)
					if contains(append(l.Funcs, l.Fmtfuncs...), f.Sel.Name) && len(ret.Args) > 0 {
						ex := ret.Args[0]

						if v, ok := ex.(*ast.BasicLit); ok && v.Kind == token.STRING {
							buf := bytes.NewBuffer([]byte{})
							printer.Fprint(buf, l.Fset, v)
							logrus.Debugf("\n   found a string:\n%s", buf.String())

							data, err := strconv.Unquote(v.Value)
							if err != nil {
								logrus.Fatal(err)
								return true
							}

							itemName, ok := noDupStrings[data]
							if !ok {
								dataCount[name]++
								itemName = name + ":" + strconv.Itoa(dataCount[name])
								noDupStrings[data] = itemName

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

								dataNames[name] = append(dataNames[name], itemName)
							}

							args := []ast.Expr{
								&ast.Ident{
									Name: "lang",
								},
								&ast.BasicLit{
									Kind:  token.STRING,
									Value: strconv.Quote(itemName),
								},
							}

							methToCall := "Trnl"
							if contains(l.Fmtfuncs, f.Sel.Name) { // is a format call
								methToCall = "Trnlf"
								dataNew, mapData, needStrconv := parseFmtString([]rune(data), ret)
								needStrconvImport = needStrconv

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
								// todo: remove duplicate for loop
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

							ret.Args = []ast.Expr{&ast.CallExpr{
								Fun: &ast.SelectorExpr{
									X: &ast.Ident{
										Name: "goloc",
									},
									Sel: &ast.Ident{
										Name: methToCall,
									},
								},
								Args: args,
							}}

							cursor.Replace(ret)
							needsImporting = true
							needsSetting = true
							return false

							// if not a string, but a binop:
						} else if v2, ok := ex.(*ast.BinaryExpr); ok && v2.Op == token.ADD {
							// note: plz reformat not to use adds
							logrus.Debug("\n   found a binary expr instead of str; fix your code")

						} else {
							logrus.Debugf("\n   found something else: %T", ex)
						}
					} else if prev, ok := f.X.(*ast.Ident); ok && prev.Name == "goloc" {
						if f.Sel.Name == "Trnl" || f.Sel.Name == "Trnlf" {
							if arg, ok := ret.Args[1].(*ast.BasicLit); ok && arg.Kind == token.STRING { // possible OOB
								val, err := strconv.Unquote(arg.Value)
								if err != nil {
									logrus.Fatal(err)
									return true
								}
								itemName, ok := noDupStrings[data[l.DefaultLang.String()][val].Value]
								if ok {
									val = itemName
								} else {
									noDupStrings[data[l.DefaultLang.String()][val].Value] = val
									// add curr data to the new data (this will remove unused vals)
									for lang := range newData {
										currVal, ok := data[lang][val]
										if !ok {
											defLangVal := data[l.DefaultLang.String()][val]
											currVal = Value{
												Id:      defLangVal.Id,
												Name:    defLangVal.Name,
												Value:   "",
												Comment: defLangVal.Value,
											}
										}
										newData[lang][name][val] = currVal
									}
									dataNames[name] = append(dataNames[name], val)
								}

								arg.Value = strconv.Quote(val)
								cursor.Replace(n)
								return false
							}
						} else if f.Sel.Name == "Add" || f.Sel.Name == "Addf" {
							if v, ok := ret.Args[0].(*ast.BasicLit); ok {
								data, err := strconv.Unquote(v.Value)
								if err != nil {
									logrus.Fatal(err)
									return true
								}

								itemName, ok := noDupStrings[data]
								if !ok {
									dataCount[name]++
									itemName = name + ":" + strconv.Itoa(dataCount[name])
									noDupStrings[data] = itemName

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

									dataNames[name] = append(dataNames[name], itemName)
								}

								args := []ast.Expr{
									&ast.Ident{
										Name: "lang",
									},
									&ast.BasicLit{
										Kind:  token.STRING,
										Value: strconv.Quote(itemName),
									},
								}
								methToCall := "Trnl"
								if f.Sel.Name == "Addf" {
									methToCall = "Trnlf"
									dataNew, mapData, needStrconv := parseFmtString([]rune(data), ret)
									needStrconvImport = needStrconv

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
									// todo: this is the same loop as earlier, simply resetting data
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

								ret = &ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X: &ast.Ident{
											Name: "goloc",
										},
										Sel: &ast.Ident{
											Name: methToCall,
										},
									},
									Args: args,
								}
								cursor.Replace(ret)
								needsImporting = true
								needsSetting = true
								return false
							}
						}
					}
				}
			}

			return true
		},
		/*post*/
		func(cursor *astutil.Cursor) bool {
			if ret, ok := cursor.Node().(*ast.FuncDecl); ok && needsSetting {
				if initExists && ret.Name.Name == "init" && needsImporting {
					if !initHasLoad(ret, name) {
						ret.Body.List = append(ret.Body.List, cntnt)
						cursor.Replace(ret)
					}
				}

				if len(ret.Body.List) == 0 {
					return true // do nothing
				} else if ass, ok := ret.Body.List[0].(*ast.AssignStmt); ok {
					// todo: stronger check
					if i, ok := ass.Lhs[0].(*ast.Ident); ok && i.Name == "lang" { // check/update generator
						return true // continue and ignore
					}
				}

				logrus.Debugln("add lang to " + name)
				ret.Body.List = append([]ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{
							&ast.Ident{
								Name: "lang",
							},
						},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{
							&ast.CallExpr{
								Fun: &ast.Ident{
									Name: "getLang", // todo: parameterise
								},
								Args: []ast.Expr{
									&ast.Ident{
										Name: "u",
									},
								}, // todo figure this bit out
							},
						},
					},
				}, ret.Body.List...)
				cursor.Replace(ret)
				needsSetting = false
			}
			return true
		},
	)

	astutil.Apply(node, func(cursor *astutil.Cursor) bool {
		return true
	}, func(cursor *astutil.Cursor) bool {
		if d, ok := cursor.Node().(*ast.GenDecl); ok && d.Tok == token.IMPORT && !initExists && needsImporting {
			v := &ast.FuncDecl{
				Name: &ast.Ident{
					Name: "init",
				},
				Type: &ast.FuncType{
					Params: &ast.FieldList{
						List: []*ast.Field{},
					},
					Results: nil,
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						cntnt,
					},
					Rbrace: 0,
				},
			}
			cursor.InsertAfter(v)
		} else if ret, ok := cursor.Node().(*ast.FuncDecl); ok && needsSetting {
			if initExists && ret.Name.Name == "init" && needsImporting {
				if !initHasLoad(ret, name) {
					ret.Body.List = append(ret.Body.List, cntnt)
					cursor.Replace(ret)
				}
			}
		}
		return true
	})

	if needsImporting {
		astutil.AddImport(l.Fset, node, "github.com/PaulSonOfLars/goloc")
		ast.SortImports(l.Fset, node)
	}

	if needStrconvImport {
		astutil.AddImport(l.Fset, node, "strconv")
		ast.SortImports(l.Fset, node)
	}

	out := os.Stdout
	if l.Apply {
		f, err := os.Create(name)
		if err != nil {
			logrus.Fatal(err)
			return
		}
		defer f.Close()
		// set file output
		out = f

	}
	if err := format.Node(out, l.Fset, node); err != nil {
		logrus.Fatal(err)
		return
	}
	if err := l.saveMap(newData, dataNames); err != nil {
		logrus.Fatal(err)
		return
	}
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

// todo: simplify the newData structure
func (l *Locer) saveMap(newData map[string]map[string]map[string]Value, dataNames map[string][]string) error {
	for lang, filenameMap := range newData {
		for name, data := range filenameMap {
			if len(dataNames[name]) == 0 {
				continue
			}

			var xmlOutput Translation
			for _, k := range dataNames[name] {
				langData := data[k]
				xmlOutput.Rows = append(xmlOutput.Rows, langData)
			}
			xmlOutput.Counter = dataCount[name]

			err := func() error {
				// TODO: other filetypes than xml
				enc := xml.NewEncoder(os.Stdout)
				if l.Apply {
					// TODO: choose translationDir
					xmlName := strings.TrimSuffix(path.Join(translationDir, lang, name), path.Ext(name)) + ".xml"
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
					enc = xml.NewEncoder(f)
				}
				enc.Indent("", "    ")
				if err := enc.Encode(xmlOutput); err != nil {
					return err
				}
				return nil
			}()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
func (l *Locer) Create(args []string, lang language.Tag) {
	err := filepath.Walk(path.Join(translationDir, l.DefaultLang.String()),
		func(fpath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()
			dec := xml.NewDecoder(f)
			var xmlData Translation
			err = dec.Decode(&xmlData)
			if err != nil {
				return err
			}

			for i := 0; i < len(xmlData.Rows); i++ {
				xmlData.Rows[i].Comment = xmlData.Rows[i].Value
				xmlData.Rows[i].Value = ""
			}

			filename := strings.Replace(fpath, sep(l.DefaultLang.String()), sep(lang.String()), 1)

			if err := os.MkdirAll(path.Dir(filename), 0755); err != nil {
				return err
			}

			newF, err := os.Create(filename)
			if err != nil {
				return err
			}
			defer newF.Close()
			enc := xml.NewEncoder(newF)
			enc.Indent("", "    ")
			if err := enc.Encode(xmlData); err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		logrus.Fatal(err)
	}
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
