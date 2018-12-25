package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"golang.org/x/tools/go/ast/astutil"
	"os"
	"strconv"
)

type row struct {
	XMLName xml.Name `xml:"person"`
	Id      int      `xml:"id,attr"`
	Name    string   `xml:"name,attr"`
	Value   string   `xml:"value"`
	Comment string   `xml:",comment"`
}

type locer struct {
	funcs       []string
	fmtfuncs    []string
	checked     map[string]struct{}
	nodes       map[string]toEdit
	orderedVals []string
	generator   string
	fset        *token.FileSet
}

type toEdit struct {
	value      *ast.BasicLit
	methodBody *ast.FuncDecl
	methCall   *ast.CallExpr
}

var apply = false

func main() {
	l := &locer{
		checked: make(map[string]struct{}),
		nodes:   make(map[string]toEdit),
		fset:    token.NewFileSet(),
	}
	verbose := false

	rootCmd := cobra.Command{
		Use:                    "goloc",
		Short:                  "Extract strings for i18n of your go tools",
		Long:                   "Simple i18n tool to allow for extracting all your i18n strings into manageable files, and load them back after.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				logrus.SetLevel(logrus.DebugLevel)
			} else {
				logrus.SetLevel(logrus.InfoLevel)
			}
		},
	}
	rootCmd.PersistentFlags().StringSliceVar(&l.funcs, "funcs", nil, "")
	rootCmd.PersistentFlags().StringVar(&l.generator, "generator", "", "")
	rootCmd.PersistentFlags().StringSliceVar(&l.fmtfuncs, "fmtfuncs", nil, "")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbosity")
	rootCmd.PersistentFlags().BoolVarP(&apply, "apply", "a", false, "apply -> save to file")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "inspect",
		Short: "Run an analyse all appropriate strings in specified files",
		Run: func(cmd *cobra.Command, args []string) {
			if err := l.handle(args, l.inspect); err != nil {
				logrus.Fatal(err)
			}
		},
	})

	//rootCmd.AddCommand(&cobra.Command{
	//	Use:   "extract",
	//	Short: "extract all strings",
	//	Run: func(cmd *cobra.Command, args []string) {
	//		l.handle(args, l.inspect)
	//		l.extract()
	//	},
	//})

	rootCmd.AddCommand(&cobra.Command{
		Use: "extract",
		Short: "extract all strings",
		Run: func(cmd *cobra.Command, args []string) {
			l.handle(args, l.fix)
		},
	})

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func (l *locer) handle(args []string, hdnl func(*ast.File)) error {
	if len(args) == 0 {
		logrus.Debugln("No input provided.")
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
			nodes, err := parser.ParseDir(l.fset, arg, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			for _, n := range nodes {
				for _, f := range n.Files {
					if _, ok := l.checked[f.Name.Name]; ok {
						continue // todo: check for file name clashes in diff packages?
					}
					l.checked[f.Name.Name] = struct{}{}
					hdnl(f)
				}
			}
		case mode.IsRegular():
			// do file stuff
			logrus.Debugln("file input")
			node, err := parser.ParseFile(l.fset, arg, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			if _, ok := l.checked[node.Name.Name]; ok {
				continue // todo: check for file name clashes in diff packages?
			}
			l.checked[node.Name.Name] = struct{}{}
			hdnl(node)
		}
	}
	return nil
}

// TODO: pass map, to avoid reiteration every time
// TODO: remove dup code with the fix() method
func (l *locer) inspect(node *ast.File) {
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
				for _, x := range append(l.funcs, l.fmtfuncs...) {
					if x == f.Sel.Name {
						isValid = true
						logrus.Debug("\n  found call named " + f.Sel.Name)
						logrus.Debugf("\n   %d", l.fset.Position(f.Pos()).Line)
						break
					}
				}
			}

			if isValid && len(ret.Args) > 0 {
				ex := ret.Args[0]

				if v, ok := ex.(*ast.BasicLit); ok && v.Kind == token.STRING {
					buf := bytes.NewBuffer([]byte{})
					printer.Fprint(buf, l.fset, v)
					logrus.Debugf("\n   found a string:\n%s", buf.String())

					counter++
					name := l.fset.File(v.Pos()).Name() + ":" + strconv.Itoa(counter)
					l.orderedVals = append(l.orderedVals, name)
					l.nodes[name] = toEdit{
						value:      v,
						methodBody: inMeth,
						methCall:   ret,
					}

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

func (l *locer) fix(node *ast.File) {
	var xmlOutput []row
	var counter int
	var inMeth *ast.FuncDecl
	var isSet bool
	var needsImporting bool
	astutil.Apply(node,
		/*pre*/ func(cursor *astutil.Cursor) bool {
			n := cursor.Node()
			//fmt.Printf("%T, %+v\n", n, n)
			if ret, ok := n.(*ast.FuncDecl); ok {
				inMeth = ret
				isSet = false

			} else if ret, ok := n.(*ast.CallExpr); ok {
				isValid := false
				//logrus.Debug("\n found a call ")
				//printer.Fprint(os.Stdout, fset, ret)
				methCallName := ""
				if f, ok := ret.Fun.(*ast.SelectorExpr); ok {
					logrus.Debug("\n  found call named " + f.Sel.Name)
					for _, x := range append(l.funcs, l.fmtfuncs...) {
						if x == f.Sel.Name {
							isValid = true
							methCallName = f.Sel.Name
							logrus.Debug("\n  found call named " + f.Sel.Name)
							logrus.Debugf("\n   %d", l.fset.Position(f.Pos()).Line)
							break
						}
					}
				}

				if isValid && len(ret.Args) > 0 {
					ex := ret.Args[0]

					if v, ok := ex.(*ast.BasicLit); ok && v.Kind == token.STRING {
						buf := bytes.NewBuffer([]byte{})
						printer.Fprint(buf, l.fset, v)
						logrus.Debugf("\n   found a string:\n%s", buf.String())

						counter++
						isFmt := false
						for _, v := range l.fmtfuncs {
							if methCallName == v {
								isFmt = true
								break
							}
						}

						name := node.Name.Name + ":" + strconv.Itoa(counter)
						args := []ast.Expr{
							&ast.Ident{
								Name: "lang",
							},
							&ast.BasicLit{
								Kind:  token.STRING,
								Value: strconv.Quote(name),
							},
						}

						data := v.Value[1 : len(v.Value)-1]

						if isFmt {
							// todo: extract this bit into its own func
							var mapdata []ast.Expr
							rdata := []rune(data)
							var newData []rune
							index := 1
							for i := 0; i < len(rdata); i++ {
								if rdata[i] == '%' && i+1 < len(rdata) {
									i++
									switch x := rdata[i]; x {
									case 's': // string -> no change
										mapdata = append(mapdata,
											&ast.KeyValueExpr{
												Key: &ast.BasicLit{
													Kind:  token.STRING,
													Value: strconv.Quote(strconv.Itoa(index)),
												},
												Value: ret.Args[index],
											})
									case 'd': // int
										//cmntMap[strconv.Itoa(index)] =
										mapdata = append(mapdata,
											&ast.KeyValueExpr{
												Key: &ast.BasicLit{
													Kind:  token.STRING,
													Value: strconv.Quote(strconv.Itoa(index)),
												},
												Value: &ast.CallExpr{
													Fun: &ast.SelectorExpr{
														X: &ast.Ident{
															Name: "strconv",
															Obj:  nil,
														},
														Sel: &ast.Ident{
															Name: "Itoa",
															Obj:  nil,
														},
													},
													Args: []ast.Expr{ret.Args[index]},
												},
											})
									case 't': // bool
										mapdata = append(mapdata,
											&ast.KeyValueExpr{
												Key: &ast.BasicLit{
													Kind:  token.STRING,
													Value: `"` + strconv.Itoa(index) + `"`,
												},
												Value: &ast.CallExpr{
													Fun: &ast.SelectorExpr{
														X: &ast.Ident{
															Name: "strconv",
															Obj:  nil,
														},
														Sel: &ast.Ident{
															Name: "FormatBool",
															Obj:  nil,
														},
													},
													Args: []ast.Expr{ret.Args[index]},
												},
											})
										//case 'p': // pointer (wtaf)
										//strconv
									default:
										logrus.Fatalf("no way to handle '%s' formatting yet", string(x))
									}
									newData = append(newData, []rune("{"+strconv.Itoa(index)+"}")...)
									index++
								} else {
									newData = append(newData, rdata[i])
								}
							}

							data = string(newData)
							args = append(args, &ast.CompositeLit{
								Type: &ast.MapType{
									Key: &ast.BasicLit{
										Kind:  token.STRING,
										Value: `"string"`,
									},
									Value: &ast.BasicLit{
										Kind:  token.STRING,
										Value: `"string"`,
									},
								},
								Elts: mapdata,
							})
						}

						xmlOutput = append(xmlOutput, row{
							Id:      counter,
							Name:    name,
							Value:   data,
							Comment: name,
						})

						ret.Args = []ast.Expr{&ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X: &ast.Ident{
									Name: "goloc",
								},
								Sel: &ast.Ident{
									Name: "Trnl",
								},
							},
							Args: args,
						}}

						cursor.Replace(ret)
						needsImporting = true

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
			if ret, ok := cursor.Node().(*ast.FuncDecl); ok && !isSet {
				if len(ret.Body.List) == 0 {
					return true // do nothing
				} else if ass, ok := ret.Body.List[0].(*ast.AssignStmt); ok {
					// todo: stronger check
					if i, ok := ass.Lhs[0].(*ast.Ident); ok && i.Name == "lang" { // check/update generator
						return true // end
					}
				}

				logrus.Debugln("add lang to " + ret.Name.Name)
				ret.Body.List = append([]ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{
							&ast.Ident{
								NamePos: 0,
								Name:    "lang",
							}},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{
							&ast.CallExpr{
								Fun: &ast.Ident{
									Name: "eng", // todo: parameterise
								},
								Args: []ast.Expr{}, // todo figure this bit out
							},
						},
					},
				}, ret.Body.List...)
				cursor.Replace(ret)
			}
			return true

		},
	)

	if needsImporting {
		astutil.AddImport(l.fset, node, "github.com/PaulSonOfLars/goloc")
		ast.SortImports(l.fset, node)
	}

	out := os.Stdout
	enc := xml.NewEncoder(os.Stdout)
	if apply {
		f, err := os.Open(node.Name.Name)
		if err != nil {
			logrus.Fatal(err)
			return
		}
		defer f.Close()
		// set file output
		out = f

		f2, err := os.Open("eng/" + node.Name.Name)
		if err != nil {
			logrus.Fatal(err)
			return
		}
		defer f2.Close()
		// set encoding output
		enc = xml.NewEncoder(f2)
	}
	printer.Fprint(out, l.fset, node)
	enc.Indent("", "    ")
	enc.Encode(xmlOutput)

}

func Trnl() {
	// todo: load from thingy and translate
}
