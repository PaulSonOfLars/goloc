package goloc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/tools/go/ast/astutil"
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
		Logger.Error("No input provided.")
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
			Logger.Debug("directory input")
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
			Logger.Debug("file input")
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
	Logger.Info("the following have been checked:")
	for k := range l.Checked {
		Logger.Info("  " + k)
	}
	return nil
}

// TODO: remove dup code with the fix() method
func (l *Locer) Inspect(node *ast.File) {
	var counter int
	// var inMeth *ast.FuncDecl
	ast.Inspect(node, func(n ast.Node) bool {
		// if ret, ok := n.(*ast.FuncDecl); ok {
		//	inMeth = ret
		//
		// } else
		if ret, ok := n.(*ast.CallExpr); ok {
			Logger.Debug("\n found a call ")
			// printer.Fprint(os.Stdout, fset, ret)
			if f, ok := ret.Fun.(*ast.SelectorExpr); ok {
				Logger.Debug("\n  found call named " + f.Sel.Name)
				if contains(append(l.Funcs, l.Fmtfuncs...), f.Sel.Name) && len(ret.Args) > 0 {
					ex := ret.Args[0]

					if v, ok := ex.(*ast.BasicLit); ok && v.Kind == token.STRING {
						buf := bytes.NewBuffer([]byte{})
						printer.Fprint(buf, l.Fset, v)
						Logger.Debugf("\n   found a string:\n%s", buf.String())

						counter++
						name := l.Fset.File(v.Pos()).Name() + ":" + strconv.Itoa(counter)
						l.OrderedVals = append(l.OrderedVals, name)

					} else if v2, ok := ex.(*ast.BinaryExpr); ok && v2.Op == token.ADD {
						// note: plz reformat not to use adds
						Logger.Debug("\n   found a binary expr instead of str; fix your code")
						// v, ok := v2.X.(*ast.BasicLit)
						// v, ok := v2.Y.(*ast.BasicLit)

					} else {
						Logger.Debugf("\n   found something else: %T", ex)

					}
				}
			}
		}
		return true
	})
	Logger.Debug()
}

var newData map[string]map[string]map[string]Value // locale:(filename:(trigger:Value))
var newDataNames map[string][]string               // filename:[]newtriggers
var noDupStrings map[string]string                 // map of currently loaded strings, to avoid duplicates and reduce translation efforts

// todo: ensure import works as expected
func (l *Locer) Fix(node *ast.File) {
	name := l.Fset.File(node.Pos()).Name()
	loadModuleExpr := &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "goloc"},
				Sel: &ast.Ident{Name: "Load"},
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
	Logger.Debug("module count at", dataCount[name])
	newData = make(map[string]map[string]map[string]Value) // locale:(filename:(trigger:Value))
	newDataNames = make(map[string][]string)               // filename:[]newtriggers
	noDupStrings = make(map[string]string)                 // map of currently loaded strings, to avoid duplicates and reduce translation efforts

	// make sure default language is loaded
	newData[l.DefaultLang.String()] = make(map[string]map[string]Value)
	newData[l.DefaultLang.String()][name] = make(map[string]Value)
	// initialise set for all other languages
	for k := range data { // initialise all languages
		newData[k] = make(map[string]map[string]Value)
		newData[k][name] = make(map[string]Value)
	}

	var needsLangSetting bool  // method needs the lang := arg
	var needGolocImport bool   // goloc needs importing
	var needStrconvImport bool // need to import strconv
	var initExists bool        // does init method exist

	// should return to node?
	astutil.Apply(node,
		/*pre*/
		func(cursor *astutil.Cursor) bool {
			n := cursor.Node()
			// get info on init call.
			if ret, ok := n.(*ast.FuncDecl); ok {
				if ret.Name.Name == "init" {
					initExists = true
				}

				// Check method calls
			} else if callExpr, ok := n.(*ast.CallExpr); ok {
				// determine if method is one of the validated ones
				if funcCall, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					Logger.Debug("\n  found random call named " + funcCall.Sel.Name)

					// if valid and has args, check first arg (which should be a string)
					if contains(append(l.Funcs, l.Fmtfuncs...), funcCall.Sel.Name) && len(callExpr.Args) > 0 {
						firstArg := callExpr.Args[0]

						if litItem, ok := firstArg.(*ast.BasicLit); ok && litItem.Kind == token.STRING {
							buf := bytes.NewBuffer([]byte{})
							printer.Fprint(buf, l.Fset, litItem)
							Logger.Debugf("\n   found a string in funcname %s:\n%s", funcCall.Sel.Name, buf.String())

							args, needStrconvImportNew := l.injectTran(name, callExpr, funcCall, litItem)

							funcCall.Sel.Name = l.getUnFmtFunc(funcCall.Sel.Name)
							callExpr.Fun = funcCall
							callExpr.Args = []ast.Expr{args}
							cursor.Replace(callExpr)
							needStrconvImport = needStrconvImportNew
							needGolocImport = true
							needsLangSetting = true
							return false

							// if not a string, but a binop:
						} else if binExpr, ok := firstArg.(*ast.BinaryExpr); ok && binExpr.Op == token.ADD {
							// note: plz reformat not to use adds
							Logger.Debug("\n   found a binary expr instead of str; fix your code")

						} else {
							Logger.Debugf("\n   found something else: %T", firstArg)
						}
					} else if caller, ok := funcCall.X.(*ast.Ident); ok && caller.Name == "goloc" {
						// has already been translated, check if it isn't duplicated.
						if funcCall.Sel.Name == "Trnl" || funcCall.Sel.Name == "Trnlf" {
							if arg, ok := callExpr.Args[1].(*ast.BasicLit); ok && arg.Kind == token.STRING { // possible OOB
								val, err := strconv.Unquote(arg.Value)
								if err != nil {
									Logger.Fatal(err)
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
											// add to old data list, so its added at the start and offsets aren't changed.
										}
										newData[lang][name][val] = currVal
									}
								}

								arg.Value = strconv.Quote(val)
								cursor.Replace(n)
								return false
							}
						} else if funcCall.Sel.Name == "Add" || funcCall.Sel.Name == "Addf" {
							if v, ok := callExpr.Args[0].(*ast.BasicLit); ok {
								buf := bytes.NewBuffer([]byte{})
								printer.Fprint(buf, l.Fset, v)
								Logger.Debugf("\n   found a string to add via Add(f):\n%s", buf.String())

								callExpr, needStrconvImport = l.injectTran(name, callExpr, funcCall, v)

								cursor.Replace(callExpr)
								needGolocImport = true
								needsLangSetting = true
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
			if FuncDecl, ok := cursor.Node().(*ast.FuncDecl); ok && needsLangSetting {
				if initExists && FuncDecl.Name.Name == "init" && needGolocImport {
					if !initHasLoad(FuncDecl, name) {
						FuncDecl.Body.List = append(FuncDecl.Body.List, loadModuleExpr)
						cursor.Replace(FuncDecl)
					}
				}

				if len(FuncDecl.Body.List) == 0 {
					return true // do nothing
				} else if ass, ok := FuncDecl.Body.List[0].(*ast.AssignStmt); ok {
					// todo: stronger check
					if i, ok := ass.Lhs[0].(*ast.Ident); ok && i.Name == "lang" { // check/update generator
						return true // continue and ignore
					}
				}

				Logger.Debug("adding lang to " + name)
				FuncDecl.Body.List = append([]ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: "lang"}},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{
							&ast.CallExpr{
								Fun:  &ast.Ident{Name: "getLang"},       // todo: parameterise
								Args: []ast.Expr{&ast.Ident{Name: "u"}}, // todo: figure this bit out
							},
						},
					},
				}, FuncDecl.Body.List...)
				cursor.Replace(FuncDecl)
				needsLangSetting = false
			}
			return true
		},
	)

	astutil.Apply(node, func(cursor *astutil.Cursor) bool {
		return true
	}, func(cursor *astutil.Cursor) bool {
		if d, ok := cursor.Node().(*ast.GenDecl); ok && d.Tok == token.IMPORT && !initExists && needGolocImport {
			v := &ast.FuncDecl{
				Name: &ast.Ident{Name: "init"},
				Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{}}},
				Body: &ast.BlockStmt{List: []ast.Stmt{loadModuleExpr}},
			}
			cursor.InsertAfter(v)
		} else if ret, ok := cursor.Node().(*ast.FuncDecl); ok && needsLangSetting {
			if initExists && ret.Name.Name == "init" && needGolocImport {
				if !initHasLoad(ret, name) {
					ret.Body.List = append(ret.Body.List, loadModuleExpr)
					cursor.Replace(ret)
				}
			}
		}
		return true
	})

	if needGolocImport {
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
			Logger.Fatal(err)
			return
		}
		defer f.Close()
		// set file output
		out = f

	}
	if err := format.Node(out, l.Fset, node); err != nil {
		Logger.Fatal(err)
		return
	}
	if err := l.saveMap(newData, newDataNames); err != nil {
		Logger.Fatal(err)
		return
	}
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
			newF.WriteString(xml.Header)
			enc := xml.NewEncoder(newF)
			enc.Indent("", "    ")
			if err := enc.Encode(xmlData); err != nil {
				return err
			}
			newF.WriteString("\n")
			return nil
		})
	if err != nil {
		Logger.Fatal(err)
	}
}

func (l *Locer) CheckAll() error {
	LoadAll(l.DefaultLang.String())
	fmt.Println(len(data))
	fmt.Println(len(data[l.DefaultLang.String()]))

	for k := range data {
		lang := language.Make(k)
		if err := l.check(lang); err != nil {
			return err
		}
	}
	return nil
}

func (l *Locer) Check(lang language.Tag) error {
	LoadLangAll(l.DefaultLang.String())
	LoadLangAll(lang.String())
	fmt.Println(len(data))
	fmt.Println(len(data[l.DefaultLang.String()]))

	return l.check(lang)
}

func (l *Locer) check(lang language.Tag) error {
	if lang == l.DefaultLang { // don't check english
		return nil
	}
	// TODO: check all inputs contain correct {} tags
	// TODO: check all inputs have the html tags escaped right; & < > ' "
	// TODO: check all inputs have valid newlines
	// TODO: check all inputs have start/end whitespace
	// TODO: investigate changing decoder
	return nil
}

func (l *Locer) getUnFmtFunc(name string) string {
	if !strings.HasSuffix(name, "f") {
		// not a formatting function; all ok.
		return name
	}
	for _, f := range l.Funcs {
		if f+"f" == name {
			// found simple func; return.
			return f
		}
	}
	// no result found; return current.
	return name
}
