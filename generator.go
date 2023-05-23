package derive

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/types"
	"html/template"
	"io"
	"os"
	"strings"

	"github.com/Masterminds/sprig/v3"
	"github.com/wule61/derive/i18n"

	"golang.org/x/tools/go/packages"
)

type File struct {
	Pkg      *Package // Package to which this file belongs.
	FileName string
	AstFile  *ast.File // Parsed AST.
	Types    []Type
}

type Type struct {
	TypeName string
	TypeType string
	Derives  []Derive
}

func (f *File) AddDerive(tName, tType string, derives []Derive) {
	f.Types = append(f.Types, Type{
		TypeName: tName,
		TypeType: tType,
		Derives:  derives,
	})
}

func (f *File) GenDecl(node ast.Node) bool {

	fileNode, ok := node.(*ast.File)
	if !ok {
		return true
	}

	for _, spec := range fileNode.Decls {
		gDecl, ok := spec.(*ast.GenDecl)
		if !ok {
			continue
		}
		var comments string
		if gDecl.Doc == nil || len(gDecl.Doc.List) == 0 {
			continue
		}

		if gDecl.Doc != nil {
			for _, comment := range gDecl.Doc.List {
				comments += comment.Text
			}
		}

		var typ string
		var tType string
		if spec := gDecl.Specs[0]; spec != nil {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			typ = typeSpec.Name.Name
			tIdent, ok := typeSpec.Type.(*ast.Ident)
			if !ok {
				continue
			}
			tType = tIdent.Name
		}

		f.AddDerive(typ, tType, ParseCommentToDerive(comments))
	}

	return false
}

type Package struct {
	Name    string // 包名
	PkgPath string
	Defs    map[*ast.Ident]types.Object // 一个包的所有定义共享
	File    []*File                     // 一个包可能有多个文件
}

type Generator struct {
	Buf bytes.Buffer // Accumulated output.
	Pkg *Package     // Package we are scanning.
}

func (g *Generator) AddPackage(pkg *packages.Package) {
	g.Pkg = &Package{
		Name:    pkg.Name,
		PkgPath: pkg.PkgPath,
		Defs:    pkg.TypesInfo.Defs,
		File:    make([]*File, len(pkg.Syntax)),
	}

	for i, file := range pkg.Syntax {
		g.Pkg.File[i] = &File{
			Pkg:      g.Pkg,
			FileName: pkg.Fset.File(file.Pos()).Name(),
			AstFile:  file,
		}
	}
}

func (g *Generator) Generate(file *File) {

	var buffer bytes.Buffer
	buffer.WriteString(`// Code generated by derive; DO NOT EDIT.`)
	buffer.Write([]byte("\n\n"))
	buffer.WriteString(`package ` + file.Pkg.Name)
	// import
	buffer.Write([]byte("\n\n"))
	buffer.WriteString("import (\n \"github.com/wule61/derive/utils\" \n \"fmt\" \n)")

	for _, Type := range file.Types {
		for _, derive := range Type.Derives {
			if derive.Name == "i18n" {
				data := i18n.TransFnTplData{
					Type: Type.TypeName,
					Code: i18n.Code{},
				}
				for _, v := range derive.Params {
					if v.Name == "code" {
						data.Code = i18n.Code{
							Type:  Type.TypeType,
							Value: v.Value,
						}
						continue
					}
					if v.Name == "zh-HK" {
						data.DefaultLang = i18n.Lang{
							Lang:  v.Name,
							Value: v.Value,
						}
					}

					data.Langs = append(data.Langs, i18n.Lang{
						Lang:  v.Name,
						Value: v.Value,
					})
				}

				buffer.Write([]byte("\n\n"))
				buffer.WriteString(fmt.Sprintf("// %v_ %v [%v]", Type.TypeName, data.DefaultLang.Value, data.Code.Value))
				buffer.Write([]byte("\n"))
				buffer.WriteString(fmt.Sprintf("var %v_ %v = %d", Type.TypeName, Type.TypeName, data.Code.Value))
				buffer.Write([]byte("\n\n"))

				tmpl, err := template.New("i18n_trans_fn").Funcs(sprig.FuncMap()).Parse(i18n.TransFnTpl)
				if err != nil {
					panic(err)
				}

				err = tmpl.Execute(&buffer, data)
				if err != nil {
					panic(err)
				}

				src, err := format.Source(buffer.Bytes())
				if err != nil {
					panic(err)
				}

				err = WriteToFile(g.GetFileName(file.FileName, derive.Name), src)
				if err != nil {
					panic(err)
				}
			}
		}
	}
}

// GetFileName  获取要生成的文件名称
func (g *Generator) GetFileName(fileName, deriveName string) string {
	arr := strings.Split(fileName, ".")
	if len(arr) > 1 {
		fileName = strings.Join(arr[:len(arr)-1], ".")
	}

	return fileName + "_" + deriveName + ".go"

}

func WriteToFile(fileName string, content []byte) error {
	f, err := os.OpenFile(fileName, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	n, _ := f.Seek(0, io.SeekEnd)
	_, err = f.WriteAt(content, n)
	return err
}
