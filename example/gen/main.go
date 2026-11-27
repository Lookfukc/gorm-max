//go:build ignore

package main

import (
	"log"

	"gorm-max/example/model"
	"gorm-max/gormmaxgen"
)

// 生成示例的字段集。运行方式(工作目录需在 example/ 下,Out 相对 CWD):
//
//	go generate ./...   // 或
//	go run ./gen/main.go
func main() {
	if err := gormmaxgen.Generate(gormmaxgen.Config{
		Out:    "query/fields_gen.go",
		Models: []any{&model.User{}},
	}); err != nil {
		log.Fatal(err)
	}
}
