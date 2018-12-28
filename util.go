package goloc

import (
	"github.com/sirupsen/logrus"
	"go/ast"
	"go/token"
	"strconv"
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
	return newData, mapData, needStrconv
}
