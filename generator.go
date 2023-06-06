package derive

import (
	"bytes"
	"go/ast"
	"go/types"
	"io"
	"os"
	"strings"

	"log"

	"github.com/dave/jennifer/jen"
	"github.com/samber/lo"
	"github.com/wule61/derive/i18n"
	"golang.org/x/tools/go/packages"
)

type File struct {
	Pkg      *Package // Package to which this file belongs.
	FileName string
	AstFile  *ast.File // Parsed AST.
	Types    []DerivedMethodsForType
}

type DerivedMethodsForType struct {
	TypeName string
	TypeType string
	Derives  []DeriveType
}

func (f *File) AddDerive(tName, tType string, derives []DeriveType) {
	f.Types = append(f.Types, DerivedMethodsForType{
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

		var typeName string
		var basicType string
		if spec := gDecl.Specs[0]; spec != nil {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			typeName = typeSpec.Name.Name
			switch typeSpec.Type.(type) {
			case *ast.StructType: // type xxx struct

			case *ast.Ident: // type xxx int
				basicType = typeSpec.Type.(*ast.Ident).Name
			default:
				continue
			}
		}

		f.AddDerive(typeName, basicType, ParseCommentToDerive(comments))
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

	f := jen.NewFile(file.Pkg.Name)
	f.HeaderComment("Code generated by derive; DO NOT EDIT.")

	for _, Type := range file.Types {
		for _, derive := range Type.Derives {
			if derive.Name == "i18n" {
				data := i18n.Data{
					Type: Type.TypeName,
					Code: i18n.ErrorCode{},
				}

				fields := lo.KeyBy(derive.Args, func(field Field) string {
					return field.Name
				})

				if code, ok := fields["code"]; ok {
					data.Code = i18n.ErrorCode{
						Type:  Type.TypeType,
						Value: code.Value,
					}
				}

				for _, v := range derive.Args {
					if v.Name == "code" || v.Name == "default" {
						continue
					}
					data.Langs = append(data.Langs, i18n.Lang{
						Lang:  v.Name,
						Value: v.Value,
					})
				}

				if len(data.Langs) == 0 {
					continue
				}

				if defaultLang, ok := fields["default"]; ok {
					data.DefaultLang = i18n.Lang{
						Lang:  defaultLang.Value.(string),
						Value: fields[defaultLang.Value.(string)].Value,
					}
				} else {
					data.DefaultLang = data.Langs[0]
				}

				i18n.GenerateCode(data, f)
				err := WriteToFile(g.GetFileName(file.FileName, derive.Name), []byte(f.GoString()))
				if err != nil {
					panic(err)
				}
			} else {
				log.Printf("derive %#v not support\n", derive)
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
