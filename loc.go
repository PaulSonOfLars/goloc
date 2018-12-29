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
	//Nodes       map[string]ToEdit
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
			isValid := false
			//logrus.Debug("\n found a call ")
			//printer.Fprint(os.Stdout, fset, ret)
			if f, ok := ret.Fun.(*ast.SelectorExpr); ok {
				//logrus.Debug("\n  found call named " + f.Sel.Name)
				for _, x := range append(l.Funcs, l.Fmtfuncs...) {
					if x == f.Sel.Name {
						isValid = true
						logrus.Debug("\n  found call named " + f.Sel.Name)
						logrus.Debugf("\n   %d", l.Fset.Position(f.Pos()).Line)
						break
					}
				}
			}

			if isValid && len(ret.Args) > 0 {
				ex := ret.Args[0]

				if v, ok := ex.(*ast.BasicLit); ok && v.Kind == token.STRING {
					buf := bytes.NewBuffer([]byte{})
					printer.Fprint(buf, l.Fset, v)
					logrus.Debugf("\n   found a string:\n%s", buf.String())

					counter++
					name := l.Fset.File(v.Pos()).Name() + ":" + strconv.Itoa(counter)
					l.OrderedVals = append(l.OrderedVals, name)
					//l.Nodes[name] = ToEdit{
					//	Value:      v,
					//	MethodBody: inMeth,
					//	MethCall:   ret,
					//}

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
				&ast.Ident{
					Name: strconv.Quote(name),
				},
			},
		},
	}

	// todo: check init load works as expected

	Load(name) // load current values

	newData := make(map[string]map[string]map[string]Value) // locale:(filename:(trigger:Value))
	dataNames := make(map[string][]string)                  // filename:[]triggers

	newData[l.DefaultLang.String()] = make(map[string]map[string]Value)
	newData[l.DefaultLang.String()][name] = make(map[string]Value)
	for k := range data { // initialise all languages
		newData[k] = make(map[string]map[string]Value)
		newData[k][name] = make(map[string]Value)
	}

	var counter int            // translation counter value
	var needsSetting bool      // method needs the lang := arg
	var needsImporting bool    // goloc needs importing
	var needStrconvImport bool // need to import strconv
	var initExists bool        // does init method exist
	var initSet bool           // has the init method been set

	// should return to node?
	astutil.Apply(node,
		/*pre*/ func(cursor *astutil.Cursor) bool {
			n := cursor.Node()
			if ret, ok := n.(*ast.FuncDecl); ok {
				if ret.Name.Name == "init" {
					initExists = true
				}

			} else if ret, ok := n.(*ast.CallExpr); ok {
				isValid := false
				methCallName := ""
				if f, ok := ret.Fun.(*ast.SelectorExpr); ok {
					logrus.Debug("\n  found random call named " + f.Sel.Name)
					for _, x := range append(l.Funcs, l.Fmtfuncs...) {
						if x == f.Sel.Name {
							isValid = true
							methCallName = f.Sel.Name
							logrus.Debug("\n  found valid call named " + f.Sel.Name)
							logrus.Debugf("\n   %d", l.Fset.Position(f.Pos()).Line)
							break
						}
					}
				}

				if isValid && len(ret.Args) > 0 {
					ex := ret.Args[0]

					if v, ok := ex.(*ast.BasicLit); ok && v.Kind == token.STRING {
						buf := bytes.NewBuffer([]byte{})
						printer.Fprint(buf, l.Fset, v)
						logrus.Debugf("\n   found a string:\n%s", buf.String())

						counter++
						isFmt := false
						for _, v := range l.Fmtfuncs {
							if methCallName == v {
								isFmt = true
								break
							}
						}

						itemName := name + ":" + strconv.Itoa(counter)
						args := []ast.Expr{
							&ast.Ident{
								Name: "lang",
							},
							&ast.BasicLit{
								Kind:  token.STRING,
								Value: strconv.Quote(itemName),
							},
						}

						data := v.Value[1 : len(v.Value)-1]
						methToCall := "Trnl"
						if isFmt {
							methToCall = "Trnlf"
							newData, mapData, needStrconv := parseFmtString([]rune(data), ret)
							needStrconvImport = needStrconv

							data = string(newData)
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

						for lang := range newData {
							newData[lang][name][itemName] = Value{
								Id:      counter,
								Name:    itemName,
								Value:   data, // TODO: only set data for default english?
								Comment: itemName,
							}
						}
						dataNames[name] = append(dataNames[name], itemName)

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

					} else if v2, ok := ex.(*ast.BinaryExpr); ok && v2.Op == token.ADD {
						// note: plz reformat not to use adds
						logrus.Debug("\n   found a binary expr instead of str; fix your code")

					} else if meth, ok := ex.(*ast.CallExpr); ok {
						logrus.Debugf("\n   found a subcall method: %+v", meth)
						if fun, ok := meth.Fun.(*ast.SelectorExpr); ok && fun.X.(*ast.Ident).Name == "goloc" && (fun.Sel.Name == "Trnl" || fun.Sel.Name == "Trnlf") {
							if arg, ok := meth.Args[1].(*ast.BasicLit); ok && arg.Kind == token.STRING {
								counter++ // todo: remove duplicate counter increment
								itemName := name + ":" + strconv.Itoa(counter)
								val, err := strconv.Unquote(arg.Value)
								if err != nil {
									logrus.Fatal(err)
									return true
								}
								for lang := range newData {
									newData[lang][name][itemName] = Value{
										Id:      counter,
										Name:    itemName,
										Value:   data[lang][val].Value,
										Comment: data[lang][val].Comment,
									}
								}
								dataNames[name] = append(dataNames[name], itemName)
								arg.Value = strconv.Quote(itemName)
								cursor.Replace(n)
							}
						} else {
							logrus.Debugf("\n   found an unexpected subcall method: %T, %+v", ex, ex)
						}
					} else {
						logrus.Debugf("\n   found something else: %T", ex)
					}
				}
			}

			return true
		},
		/*post*/
		func(cursor *astutil.Cursor) bool {
			if ret, ok := cursor.Node().(*ast.FuncDecl); ok && needsSetting {
				if initExists && ret.Name.Name == "init" && !initSet && needsImporting {
					ret.Body.List = append(ret.Body.List, cntnt)
					cursor.Replace(ret)
					initSet = true
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
		if d, ok := cursor.Node().(*ast.GenDecl); ok && d.Tok == token.IMPORT && !initExists && !initSet && needsImporting {
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
			initSet = true
		} else if ret, ok := cursor.Node().(*ast.FuncDecl); ok && needsSetting {

			if initExists && ret.Name.Name == "init" && !initSet && needsImporting {
				ret.Body.List = append(ret.Body.List, cntnt)
				cursor.Replace(ret)
				initSet = true
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

// todo: simplify the newData structure
func (l *Locer) saveMap(newData map[string]map[string]map[string]Value, dataNames map[string][]string) error {
	for lang, filenameMap := range newData {
		for name, data := range filenameMap {
			var xmlOutput Translation
			for _, k := range dataNames[name] {
				langData := data[k]
				if lang != l.DefaultLang.String() { // do not set english string for non-english languages
					langData.Value = ""
				}
				xmlOutput.Rows = append(xmlOutput.Rows, langData)
			}

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
