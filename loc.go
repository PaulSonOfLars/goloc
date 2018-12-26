package goloc

import (
	"bytes"
	"encoding/xml"
	"github.com/sirupsen/logrus"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
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
	Rows []Value
}

type Value struct {
	Id      int      `xml:"id,attr"`
	Name    string   `xml:"name,attr"`
	Value   string   `xml:"value"`
	Comment string   `xml:",comment"`
}

type Locer struct {
	Funcs    []string
	Fmtfuncs []string
	Checked  map[string]struct{}
	//Nodes       map[string]ToEdit
	OrderedVals []string
	Generator   string
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

// TODO: pass map, to avoid reiteration every time
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

// TODO: decide what to do with this dead code.
// stick to astutil currently used, or the byte-level ops done here?

//func (l *locer) extract() error {
//	opened := make(map[string][]byte)
//	offset := make(map[string]int)
//	for _, v := range l.orderedVals {
//		toEdit := l.nodes[v]
//		node := toEdit.value
//		methBody := toEdit.methodBody
//		filename := l.fset.File(node.Pos()).Name()
//		ctnt, ok := opened[filename]
//		if !ok {
//			dat, err := ioutil.ReadFile(filename)
//			if err != nil {
//				return err
//			}
//			ctnt = dat
//		}
//
//		off := offset[filename]
//
//		logrus.Debugln(methBody.Name)
//		logrus.Debugln(string(ctnt[l.fset.Position(methBody.Body.Pos()).Offset+off : l.fset.Position(methBody.Body.End()).Offset+off]))
//
//		loader := fmt.Sprintf("lang := %s\n", "GetLang(u)") // todo this should be modularised
//		ctnt = append([]byte(loader), ctnt[l.fset.Position(methBody.Body.Pos()).Offset+off:l.fset.Position(methBody.Body.End()).Offset+off]...)
//
//		off += len(loader)
//
//		//fmt.Println(methBody.Body.Pos())
//		//fmt.Println(methBody.Body.End())
//
//		// todo: need to have method body access to write lang loader (+ add offset)
//		repl := ""
//		//ok := false
//		//for _, v := range l.fmtfuncs {
//		//if methCallName == v { // todo: need methcall access
//		//	ok = true
//		//	break
//		//}
//		//}
//		//if ok {
//		//	its a fmt func; generate extra
//		//} else {
//		//	repl = `"idek"`
//		//}
//		start := l.fset.Position(node.Pos())
//		end := l.fset.Position(node.End())
//
//		logrus.Debug(start, start.Offset, start.Offset+off)
//		logrus.Debug(end, end.Offset, start.Offset+off)
//		logrus.Debug(len(ctnt))
//
//		opened[filename] = append(append(ctnt[:start.Offset+off], repl...), ctnt[end.Offset+off:]...)
//		offset[filename] = off + start.Offset - end.Offset + len(repl)
//	}
//	//for _, v := range opened {
//	//	fmt.Println(string(v))
//	//}
//	return nil
//}

// todo: add a "load" call to the init() method for each loaded file
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

	var xmlOutput Translation
	var counter int
	var needsSetting bool
	var needsImporting bool
	var initExists bool
	var initSet bool
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
					logrus.Debug("\n  found call named " + f.Sel.Name)
					for _, x := range append(l.Funcs, l.Fmtfuncs...) {
						if x == f.Sel.Name {
							isValid = true
							methCallName = f.Sel.Name
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
							newData, mapData := parseFmtString([]rune(data), ret)

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

						xmlOutput.Rows = append(xmlOutput.Rows, Value{
							Id:      counter,
							Name:    itemName,
							Value:   data,
							Comment: itemName,
						})

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

	// todo: add strconv import

	out := os.Stdout
	enc := xml.NewEncoder(os.Stdout)
	if l.Apply {
		f, err := os.Create(name)
		if err != nil {
			logrus.Fatal(err)
			return
		}
		defer f.Close()
		// set file output
		out = f

		// TODO: handle cases where default isnt english
		// TODO: other filetypes than xml
		// TODO: choose translationDir
		filename := strings.TrimSuffix(path.Join(translationDir, "en-GB", name), path.Ext(name)) + ".xml"
		err = os.MkdirAll(filepath.Dir(filename), 0755)
		if err != nil {
			logrus.Fatal(err)
			return
		}
		f2, err := os.Create(filename) // todo parameterise
		if err != nil {
			logrus.Fatal(err)
			return
		}
		defer f2.Close()
		// set encoding output
		enc = xml.NewEncoder(f2)
	}
	if err := printer.Fprint(out, l.Fset, node); err != nil {
		logrus.Fatal(err)
		return
	}
	enc.Indent("", "    ")
	if err := enc.Encode(xmlOutput); err != nil {
		logrus.Fatal(err)
		return
	}
}
